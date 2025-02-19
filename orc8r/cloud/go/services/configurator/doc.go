/*
 * Copyright (c) Facebook, Inc. and its affiliates.
 * All rights reserved.
 *
 * This source code is licensed under the BSD-style license found in the
 * LICENSE file in the root directory of this source tree.
 */

// Package configurator contains the Configurator service which manages
// configuration of and relationships between logical network entities.
package configurator

const (
	// ServiceName is the name of this service
	ServiceName = "CONFIGURATOR"
	// SerdeDomain is the name of this service's serde domain
	SerdeDomain       = "config_manager"
	GatewayEntityType = "gateway"
)
