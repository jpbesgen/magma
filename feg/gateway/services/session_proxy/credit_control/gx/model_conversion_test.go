/*
Copyright (c) Facebook, Inc. and its affiliates.
All rights reserved.

This source code is licensed under the BSD-style license found in the
LICENSE file in the root directory of this source tree.
*/

package gx_test

import (
	"sort"
	"strings"
	"testing"
	"time"

	"magma/feg/gateway/policydb/mocks"
	"magma/feg/gateway/services/session_proxy/credit_control/gx"
	"magma/lte/cloud/go/protos"

	"github.com/fiorix/go-diameter/diam"
	"github.com/golang/protobuf/ptypes"
	"github.com/stretchr/testify/assert"
)

func TestReAuthRequest_ToProto(t *testing.T) {
	// Check nil, 1-element, multiple elements, and empty arrays
	monitoringKey := "monitor"
	var ratingGroup uint32 = 42
	currentTime := time.Now()
	protoTimestamp, err := ptypes.TimestampProto(currentTime)
	assert.NoError(t, err)
	in := &gx.ReAuthRequest{
		SessionID: "IMSI001010000000001-1234",
		RulesToRemove: []*gx.RuleRemoveAVP{
			{RuleNames: []string{"remove1", "remove2"}, RuleBaseNames: []string{"baseRemove1"}},
			{RuleNames: nil, RuleBaseNames: nil},
			{RuleNames: []string{"remove3"}, RuleBaseNames: []string{}},
			{RuleNames: []string{}, RuleBaseNames: []string{"baseRemove2", "baseRemove3"}},
		},
		RulesToInstall: []*gx.RuleInstallAVP{
			{RuleNames: []string{"install1", "install2"}, RuleBaseNames: []string{"baseInstall1"}, RuleDefinitions: nil},
			{
				RuleNames:     nil,
				RuleBaseNames: nil,
				RuleDefinitions: []*gx.RuleDefinition{
					{RuleName: "dynamic1", MonitoringKey: &monitoringKey, Precedence: 100, RatingGroup: &ratingGroup},
				},
			},
			{RuleNames: []string{"install3"}, RuleBaseNames: []string{}},
			{RuleNames: []string{}, RuleBaseNames: []string{"baseInstall2", "baseInstall3"}},
		},
		EventTriggers:    []gx.EventTrigger{gx.UsageReportTrigger, gx.RevalidationTimeout},
		RevalidationTime: &currentTime,
	}
	policyClient := &mocks.PolicyDBClient{}
	policyClient.On("GetRuleIDsForBaseNames", []string{"baseRemove1", "baseRemove2", "baseRemove3"}).
		Return([]string{"remove42", "remove43", "remove44"})
	policyClient.On("GetRuleIDsForBaseNames", []string{"baseInstall1"}).
		Return([]string{})
	policyClient.On("GetRuleIDsForBaseNames", []string{"baseInstall2", "baseInstall3"}).
		Return([]string{"install42", "install43"})

	actual := in.ToProto("IMSI001010000000001", "magma;1234;1234;IMSI001010000000001", policyClient)
	expected := &protos.PolicyReAuthRequest{
		SessionId:     "magma;1234;1234;IMSI001010000000001",
		Imsi:          "IMSI001010000000001",
		RulesToRemove: []string{"remove1", "remove2", "remove3", "remove42", "remove43", "remove44"},
		RulesToInstall: []*protos.StaticRuleInstall{
			&protos.StaticRuleInstall{
				RuleId: "install1",
			},
			&protos.StaticRuleInstall{
				RuleId: "install2",
			},
			&protos.StaticRuleInstall{
				RuleId: "install3",
			},
			&protos.StaticRuleInstall{
				RuleId: "install42",
			},
			&protos.StaticRuleInstall{
				RuleId: "install43",
			},
		},
		DynamicRulesToInstall: []*protos.DynamicRuleInstall{
			&protos.DynamicRuleInstall{
				PolicyRule: &protos.PolicyRule{
					Id:            "dynamic1",
					RatingGroup:   42,
					MonitoringKey: monitoringKey,
					Priority:      100,
					TrackingType:  protos.PolicyRule_OCS_AND_PCRF,
				},
			},
		},
		EventTriggers: []protos.EventTrigger{
			protos.EventTrigger_UNSUPPORTED,
			protos.EventTrigger_REVALIDATION_TIMEOUT,
		},
		RevalidationTime: protoTimestamp,
	}
	assert.Equal(t, expected, actual)
	policyClient.AssertExpectations(t)
}

func TestReAuthAnswer_FromProto(t *testing.T) {
	in := &protos.PolicyReAuthAnswer{
		SessionId: "foo",
		FailedRules: map[string]protos.PolicyReAuthAnswer_FailureCode{
			"bar": protos.PolicyReAuthAnswer_CM_AUTHORIZATION_REJECTED,
			"baz": protos.PolicyReAuthAnswer_AN_GW_FAILED,
		},
	}
	actual := (&gx.ReAuthAnswer{}).FromProto("sesh", in)

	// sort the rules so we get a deterministic test
	sortFun := func(i, j int) bool {
		first := actual.RuleReports[i]
		second := actual.RuleReports[j]

		concattedFirst := strings.Join(first.RuleNames, "") + strings.Join(first.RuleBaseNames, "")
		concattedSecond := strings.Join(second.RuleNames, "") + strings.Join(second.RuleBaseNames, "")
		return concattedFirst < concattedSecond
	}
	sort.Slice(actual.RuleReports, sortFun)

	expected := &gx.ReAuthAnswer{
		SessionID:  "sesh",
		ResultCode: diam.Success,
		RuleReports: []*gx.ChargingRuleReport{
			{RuleNames: []string{"bar"}, FailureCode: gx.CMAuthorizationRejected},
			{RuleNames: []string{"baz"}, FailureCode: gx.ANGWFailed},
		},
	}
	assert.Equal(t, expected, actual)
}

func TestRuleDefinition_ToProto(t *testing.T) {
	// Check nil, 1-element, multiple elements, and empty arrays
	monitoringKey := "monitor"
	var ratingGroup uint32 = 10
	var ruleOut *protos.PolicyRule = nil

	ruleOut = (&gx.RuleDefinition{
		RuleName:      "rgonly",
		MonitoringKey: nil,
		RatingGroup:   &ratingGroup,
	}).ToProto()
	assert.Equal(t, "", ruleOut.MonitoringKey)
	assert.Equal(t, uint32(10), ruleOut.RatingGroup)
	assert.Equal(t, protos.PolicyRule_ONLY_OCS, ruleOut.TrackingType)

	ruleOut = (&gx.RuleDefinition{
		RuleName:      "mkonly",
		MonitoringKey: &monitoringKey,
		RatingGroup:   nil,
	}).ToProto()
	assert.Equal(t, "monitor", ruleOut.MonitoringKey)
	assert.Equal(t, uint32(0), ruleOut.RatingGroup)
	assert.Equal(t, protos.PolicyRule_ONLY_PCRF, ruleOut.TrackingType)

	ruleOut = (&gx.RuleDefinition{
		RuleName:      "both",
		MonitoringKey: &monitoringKey,
		RatingGroup:   &ratingGroup,
	}).ToProto()
	assert.Equal(t, "monitor", ruleOut.MonitoringKey)
	assert.Equal(t, uint32(10), ruleOut.RatingGroup)
	assert.Equal(t, protos.PolicyRule_OCS_AND_PCRF, ruleOut.TrackingType)

	ruleOut = (&gx.RuleDefinition{
		RuleName:      "neither",
		MonitoringKey: nil,
		RatingGroup:   nil,
	}).ToProto()
	assert.Equal(t, "", ruleOut.MonitoringKey)
	assert.Equal(t, uint32(0), ruleOut.RatingGroup)
	assert.Equal(t, protos.PolicyRule_NO_TRACKING, ruleOut.TrackingType)
}
