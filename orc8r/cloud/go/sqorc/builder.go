/*
 * Copyright (c) Facebook, Inc. and its affiliates.
 * All rights reserved.
 *
 * This source code is licensed under the BSD-style license found in the
 * LICENSE file in the root directory of this source tree.
 */

package sqorc

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	"github.com/Masterminds/squirrel"
	"github.com/golang/glog"
	"github.com/lann/builder"
	"github.com/thoas/go-funk"
)

const (
	postgresDialect = "psql"
	mariaDialect    = "maria"
)

// GetSqlBuilder returns a squirrel Builder for the configured SQL dialect as
// found in the SQL_DIALECT env var.
func GetSqlBuilder() StatementBuilder {
	dialect, envFound := os.LookupEnv("SQL_DIALECT")
	// Default to postgresql
	if !envFound {
		return NewPostgresStatementBuilder()
	}

	switch strings.ToLower(dialect) {
	case postgresDialect:
		return NewPostgresStatementBuilder()
	case mariaDialect:
		return NewMariaDBStatementBuilder()
	default:
		panic(fmt.Sprintf("unsupported sql dialect %s", dialect))
	}
}

// StatementBuilder is an interface which tracks squirrel's
// StatementBuilderType with the difference that Insert returns this package's
// InsertBuilder interface type.
// This interface exists to support building DDL commands and upsert statements
// for multiple dialects.
type StatementBuilder interface {
	Select(columns ...string) squirrel.SelectBuilder
	Insert(into string) InsertBuilder
	Update(table string) squirrel.UpdateBuilder
	Delete(from string) squirrel.DeleteBuilder

	PlaceholderFormat(f squirrel.PlaceholderFormat) squirrel.StatementBuilderType
	RunWith(runner squirrel.BaseRunner) squirrel.StatementBuilderType

	// CreateTable returns a CreateTableBuilder for building DDL table creation
	// statements.
	// IMPORTANT: the returned builder will NOT respect the runner set via
	// RunWith on this StatementBuilder due to a reflection bug that's
	// tricky to chase down.
	CreateTable(name string) CreateTableBuilder

	// CreateIndex returns a CreateIndexBuilder for building index creation
	// statements.
	// IMPORTANT: the returned builder will NOT respect the runner set via
	// RunWith on this StatementBuilder due to a reflection bug that's
	// tricky to chase down.
	CreateIndex(name string) CreateIndexBuilder
}

// NewPostgresStatementBuilder returns an implementation of StatementBuilder
// for PostgreSQL dialect.
func NewPostgresStatementBuilder() StatementBuilder {
	baseBuilder := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)
	return postgresStatementBuilder{StatementBuilderType: baseBuilder}
}

// NewMariaDBStatementBuilder returns an implementation of StatementBuilder for
// MariaDB dialect.
func NewMariaDBStatementBuilder() StatementBuilder {
	baseBuilder := squirrel.StatementBuilder.PlaceholderFormat(squirrel.Question)
	return mariaDBStatementBuilder{StatementBuilderType: baseBuilder}
}

type postgresStatementBuilder struct {
	squirrel.StatementBuilderType
}

func (psb postgresStatementBuilder) Insert(into string) InsertBuilder {
	baseInsertBuilder := psb.StatementBuilderType.Insert(into)
	return postgresInsertBuilder{baseInsertBuilder}
}

func (psb postgresStatementBuilder) CreateTable(name string) CreateTableBuilder {
	// If we use psb.StatementBuilderType as the arg to CreateTableBuilder,
	// we get the following panic:
	// panic: reflect: call of reflect.Value.Set on zero Value
	// This is hard to track down so just pass builder.EmptyBuilder
	return CreateTableBuilder(builder.EmptyBuilder).
		columnTypeNames(postgresColumnTypeMap).
		Name(name)
}

func (psb postgresStatementBuilder) CreateIndex(name string) CreateIndexBuilder {
	// see comment in CreateTable about EmptyBuilder initializer
	return CreateIndexBuilder(builder.EmptyBuilder).
		Name(name)
}

type mariaDBStatementBuilder struct {
	squirrel.StatementBuilderType
}

func (msb mariaDBStatementBuilder) Insert(into string) InsertBuilder {
	baseInsertBuilder := msb.StatementBuilderType.Insert(into)
	return mariaInsertBuilder{baseInsertBuilder}
}

func (msb mariaDBStatementBuilder) CreateTable(name string) CreateTableBuilder {
	// see comment on the postgres builder about the EmptyBuilder
	return CreateTableBuilder(builder.EmptyBuilder).
		columnTypeNames(mariaColumnTypeMap).
		Name(name)
}

func (msb mariaDBStatementBuilder) CreateIndex(name string) CreateIndexBuilder {
	// see comment on postgres builder CreateTable about EmptyBuilder
	return CreateIndexBuilder(builder.EmptyBuilder).
		Name(name)
}

// InsertBuilder is an interface which tracks squirrel's InsertBuilder
// struct but returns InsertBuilder on all self-referencing returns and adds
// an OnConflict method to support upserts.
type InsertBuilder interface {
	ExecContext(ctx context.Context) (sql.Result, error)
	QueryContext(ctx context.Context) (*sql.Rows, error)
	QueryRowContext(ctx context.Context) squirrel.RowScanner
	ScanContext(ctx context.Context, dest ...interface{}) error
	PlaceholderFormat(f squirrel.PlaceholderFormat) InsertBuilder
	RunWith(runner squirrel.BaseRunner) InsertBuilder
	Exec() (sql.Result, error)
	Query() (*sql.Rows, error)
	QueryRow() squirrel.RowScanner
	Scan(dest ...interface{}) error
	ToSql() (string, []interface{}, error)
	Prefix(sql string, args ...interface{}) InsertBuilder
	Options(options ...string) InsertBuilder
	Into(from string) InsertBuilder
	Columns(columns ...string) InsertBuilder
	Values(values ...interface{}) InsertBuilder
	Suffix(sql string, args ...interface{}) InsertBuilder
	SetMap(clauses map[string]interface{}) InsertBuilder
	Select(sb squirrel.SelectBuilder) InsertBuilder

	// OnConflict builds an upsert clause for the insert query.
	// An empty value for the setValues param indicates do nothing on conflict.
	OnConflict(setValues []UpsertValue, columns ...string) InsertBuilder
}

// UpsertValue wraps a column name and updated value
type UpsertValue struct {
	Column string
	Value  interface{}
}

type postgresInsertBuilder struct {
	squirrel.InsertBuilder
}

func (pib postgresInsertBuilder) OnConflict(setValues []UpsertValue, columns ...string) InsertBuilder {
	if funk.IsEmpty(columns) {
		panic("must provide at least one column in upsert clause builder")
	}

	suffixFormat := "ON CONFLICT %s DO %s"
	colList := fmt.Sprintf("(%s)", strings.Join(columns, ", "))

	if funk.IsEmpty(setValues) {
		return pib.Suffix(fmt.Sprintf(suffixFormat, colList, "NOTHING"))
	}

	updateStr, updateArgs := setValuesToUpsertClause(setValues, true)
	return pib.Suffix(fmt.Sprintf(suffixFormat, colList, updateStr), updateArgs...)
}

func (pib postgresInsertBuilder) PlaceholderFormat(f squirrel.PlaceholderFormat) InsertBuilder {
	newDelegate := pib.InsertBuilder.PlaceholderFormat(f)
	return postgresInsertBuilder{newDelegate}
}

func (pib postgresInsertBuilder) RunWith(runner squirrel.BaseRunner) InsertBuilder {
	newDelegate := pib.InsertBuilder.RunWith(runner)
	return postgresInsertBuilder{newDelegate}
}

func (pib postgresInsertBuilder) Prefix(sql string, args ...interface{}) InsertBuilder {
	newDelegate := pib.InsertBuilder.Prefix(sql, args...)
	return postgresInsertBuilder{newDelegate}
}

func (pib postgresInsertBuilder) Options(options ...string) InsertBuilder {
	newDelegate := pib.InsertBuilder.Options(options...)
	return postgresInsertBuilder{newDelegate}
}

func (pib postgresInsertBuilder) Into(from string) InsertBuilder {
	newDelegate := pib.InsertBuilder.Into(from)
	return postgresInsertBuilder{newDelegate}
}

func (pib postgresInsertBuilder) Columns(columns ...string) InsertBuilder {
	newDelegate := pib.InsertBuilder.Columns(columns...)
	return postgresInsertBuilder{newDelegate}
}

func (pib postgresInsertBuilder) Values(values ...interface{}) InsertBuilder {
	newDelegate := pib.InsertBuilder.Values(values...)
	return postgresInsertBuilder{newDelegate}
}

func (pib postgresInsertBuilder) Suffix(sql string, args ...interface{}) InsertBuilder {
	newDelegate := pib.InsertBuilder.Suffix(sql, args...)
	return postgresInsertBuilder{newDelegate}
}

func (pib postgresInsertBuilder) SetMap(clauses map[string]interface{}) InsertBuilder {
	newDelegate := pib.InsertBuilder.SetMap(clauses)
	return postgresInsertBuilder{newDelegate}
}

func (pib postgresInsertBuilder) Select(sb squirrel.SelectBuilder) InsertBuilder {
	newDelegate := pib.InsertBuilder.Select(sb)
	return postgresInsertBuilder{newDelegate}
}

type mariaInsertBuilder struct {
	squirrel.InsertBuilder
}

func (mib mariaInsertBuilder) OnConflict(setValues []UpsertValue, columns ...string) InsertBuilder {
	if funk.IsEmpty(setValues) {
		newDelegate := mib.InsertBuilder.Options("IGNORE")
		return mariaInsertBuilder{newDelegate}
	}

	suffixFormat := "ON DUPLICATE KEY %s"
	updateStr, updateArgs := setValuesToUpsertClause(setValues, false)
	newDelegate := mib.InsertBuilder.Suffix(fmt.Sprintf(suffixFormat, updateStr), updateArgs...)
	return mariaInsertBuilder{newDelegate}
}

func (mib mariaInsertBuilder) PlaceholderFormat(f squirrel.PlaceholderFormat) InsertBuilder {
	newDelegate := mib.InsertBuilder.PlaceholderFormat(f)
	return mariaInsertBuilder{newDelegate}
}

func (mib mariaInsertBuilder) RunWith(runner squirrel.BaseRunner) InsertBuilder {
	newDelegate := mib.InsertBuilder.RunWith(runner)
	return mariaInsertBuilder{newDelegate}
}

func (mib mariaInsertBuilder) Prefix(sql string, args ...interface{}) InsertBuilder {
	newDelegate := mib.InsertBuilder.Prefix(sql, args...)
	return mariaInsertBuilder{newDelegate}
}

func (mib mariaInsertBuilder) Options(options ...string) InsertBuilder {
	newDelegate := mib.InsertBuilder.Options(options...)
	return mariaInsertBuilder{newDelegate}
}

func (mib mariaInsertBuilder) Into(from string) InsertBuilder {
	newDelegate := mib.InsertBuilder.Into(from)
	return mariaInsertBuilder{newDelegate}
}

func (mib mariaInsertBuilder) Columns(columns ...string) InsertBuilder {
	newDelegate := mib.InsertBuilder.Columns(columns...)
	return mariaInsertBuilder{newDelegate}
}

func (mib mariaInsertBuilder) Values(values ...interface{}) InsertBuilder {
	newDelegate := mib.InsertBuilder.Values(values...)
	return mariaInsertBuilder{newDelegate}
}

func (mib mariaInsertBuilder) Suffix(sql string, args ...interface{}) InsertBuilder {
	newDelegate := mib.InsertBuilder.Suffix(sql, args...)
	return mariaInsertBuilder{newDelegate}
}

func (mib mariaInsertBuilder) SetMap(clauses map[string]interface{}) InsertBuilder {
	newDelegate := mib.InsertBuilder.SetMap(clauses)
	return mariaInsertBuilder{newDelegate}
}

func (mib mariaInsertBuilder) Select(sb squirrel.SelectBuilder) InsertBuilder {
	newDelegate := mib.InsertBuilder.Select(sb)
	return mariaInsertBuilder{newDelegate}
}

func ClearStatementCacheLogOnError(cache *squirrel.StmtCache, callsite string) {
	err := cache.Clear()
	if err != nil {
		glog.Errorf("error clearing statement cache in %s: %s", callsite, err)
	}
}

func setValuesToUpsertClause(setValues []UpsertValue, writeSet bool) (string, []interface{}) {
	setParts := funk.Map(setValues, func(uv UpsertValue) string {
		return uv.Column + " = ?"
	}).([]string)
	setClause := strings.Join(setParts, ", ")

	// This is sloppy but we can make it nice if we ever have to support more
	// than just psql and mysql
	var upsertClause string
	if writeSet {
		upsertClause = fmt.Sprintf("UPDATE SET %s", setClause)
	} else {
		upsertClause = fmt.Sprintf("UPDATE %s", setClause)
	}
	return upsertClause, funk.Map(setValues, func(uv UpsertValue) interface{} { return uv.Value }).([]interface{})
}
