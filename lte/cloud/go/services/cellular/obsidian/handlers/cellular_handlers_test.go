/*
Copyright (c) Facebook, Inc. and its affiliates.
All rights reserved.

This source code is licensed under the BSD-style license found in the
LICENSE file in the root directory of this source tree.
*/

package handlers_test

import (
	"fmt"
	"testing"

	lteplugin "magma/lte/cloud/go/plugin"
	"magma/lte/cloud/go/services/cellular/obsidian/models"
	cellular_protos "magma/lte/cloud/go/services/cellular/protos"
	"magma/lte/cloud/go/services/cellular/test_utils"
	"magma/orc8r/cloud/go/obsidian/handlers"
	obsidian_test "magma/orc8r/cloud/go/obsidian/tests"
	"magma/orc8r/cloud/go/plugin"
	"magma/orc8r/cloud/go/pluginimpl"
	"magma/orc8r/cloud/go/protos"
	configurator_test_init "magma/orc8r/cloud/go/services/configurator/test_init"
	"magma/orc8r/cloud/go/services/magmad"
	magmad_protos "magma/orc8r/cloud/go/services/magmad/protos"
	magmad_test_init "magma/orc8r/cloud/go/services/magmad/test_init"

	"github.com/stretchr/testify/assert"
)

func TestGetNetworkConfigs(t *testing.T) {
	plugin.RegisterPluginForTests(t, &lteplugin.LteOrchestratorPlugin{})
	plugin.RegisterPluginForTests(t, &pluginimpl.BaseOrchestratorPlugin{})
	magmad_test_init.StartTestService(t)
	configurator_test_init.StartTestService(t)
	restPort := obsidian_test.StartObsidian(t)
	testUrlRoot := fmt.Sprintf("http://localhost:%d%s/networks", restPort, handlers.REST_ROOT)

	networkId := registerNetwork(t, "Test Network 1", "cellular_obsidian_test_network")

	// Happy path
	expectedConfig := &models.NetworkCellularConfigs{}
	protos.FillIn(test_utils.NewDefaultFDDNetworkConfig(), expectedConfig)
	marshaledCfg, err := expectedConfig.MarshalBinary()
	assert.NoError(t, err)
	expected := string(marshaledCfg)

	createConfigTestCase := obsidian_test.Testcase{
		Name:     "Create Cellular Network Config",
		Method:   "POST",
		Url:      fmt.Sprintf("%s/%s/configs/cellular", testUrlRoot, networkId),
		Payload:  expected,
		Expected: fmt.Sprintf(`"%s"`, networkId),
	}
	obsidian_test.RunTest(t, createConfigTestCase)

	happyPathTestCase := obsidian_test.Testcase{
		Name:     "Get Cellular Network Config",
		Method:   "GET",
		Url:      fmt.Sprintf("%s/%s/configs/cellular", testUrlRoot, networkId),
		Payload:  "",
		Expected: expected,
	}
	obsidian_test.RunTest(t, happyPathTestCase)

	// No good way to test invalid configs from datastore without dropping down
	// to raw magmad api/grpc or datastore fixtures, so let's skip that
	// for now
}

func TestSetTDDNetworkConfigs(t *testing.T) {
	plugin.RegisterPluginForTests(t, &lteplugin.LteOrchestratorPlugin{})
	plugin.RegisterPluginForTests(t, &pluginimpl.BaseOrchestratorPlugin{})
	magmad_test_init.StartTestService(t)
	configurator_test_init.StartTestService(t)
	testSetNetworkConfigs(t, test_utils.NewDefaultTDDNetworkConfig(), test_utils.NewDefaultTDDNetworkConfig())
}

func TestSetFDDNetworkConfigs(t *testing.T) {
	plugin.RegisterPluginForTests(t, &lteplugin.LteOrchestratorPlugin{})
	plugin.RegisterPluginForTests(t, &pluginimpl.BaseOrchestratorPlugin{})
	magmad_test_init.StartTestService(t)
	configurator_test_init.StartTestService(t)
	testSetNetworkConfigs(t, test_utils.NewDefaultFDDNetworkConfig(), test_utils.NewDefaultFDDNetworkConfig())
}

func TestSetOldTddNetworkConfigs(t *testing.T) {
	plugin.RegisterPluginForTests(t, &lteplugin.LteOrchestratorPlugin{})
	plugin.RegisterPluginForTests(t, &pluginimpl.BaseOrchestratorPlugin{})
	magmad_test_init.StartTestService(t)
	configurator_test_init.StartTestService(t)
	testSetNetworkConfigs(t, test_utils.OldTDDNetworkConfig(), test_utils.NewDefaultTDDNetworkConfig())
}

func TestSetOldFddNetworkConfigs(t *testing.T) {
	plugin.RegisterPluginForTests(t, &lteplugin.LteOrchestratorPlugin{})
	plugin.RegisterPluginForTests(t, &pluginimpl.BaseOrchestratorPlugin{})
	magmad_test_init.StartTestService(t)
	configurator_test_init.StartTestService(t)
	testSetNetworkConfigs(t, test_utils.OldFDDNetworkConfig(), test_utils.NewDefaultFDDNetworkConfig())
}

func TestSetBadNetworkConfigs(t *testing.T) {
	plugin.RegisterPluginForTests(t, &lteplugin.LteOrchestratorPlugin{})
	plugin.RegisterPluginForTests(t, &pluginimpl.BaseOrchestratorPlugin{})
	magmad_test_init.StartTestService(t)
	configurator_test_init.StartTestService(t)
	restPort := obsidian_test.StartObsidian(t)
	testUrlRoot := fmt.Sprintf("http://localhost:%d%s/networks", restPort, handlers.REST_ROOT)

	networkId := registerNetwork(t, "Test Network 1", "cellular_obsidian_test_network")

	config := test_utils.NewDefaultTDDNetworkConfig()

	// Fail RAN config check
	config.Ran.FddConfig = &cellular_protos.NetworkRANConfig_FDDConfig{
		Earfcndl: 0,
		Earfcnul: 18000,
	}
	swaggerConfig := &models.NetworkCellularConfigs{}
	protos.FillIn(config, swaggerConfig)
	marshaledCfg, err := swaggerConfig.MarshalBinary()
	assert.NoError(t, err)
	swaggerConfigString := string(marshaledCfg)

	setConfigTestCase := obsidian_test.Testcase{
		Name:                     "Set Both TDD+FDD Network Config",
		Method:                   "PUT",
		Url:                      fmt.Sprintf("%s/%s/configs/cellular", testUrlRoot, networkId),
		Payload:                  swaggerConfigString,
		Expected:                 `{"message":"Error converting config model: Only one of TDD or FDD configs can be set"}`,
		Expect_http_error_status: true,
	}
	status, _, err := obsidian_test.RunTest(t, setConfigTestCase)
	assert.Equal(t, 400, status)

	// Fail swagger validation
	config.Epc.Mcc = "abc"
	config.Ran.BandwidthMhz = 15
	protos.FillIn(config, swaggerConfig)
	marshaledCfg, err = swaggerConfig.MarshalBinary()
	assert.NoError(t, err)
	swaggerConfigString = string(marshaledCfg)

	setConfigTestCase = obsidian_test.Testcase{
		Name:                     "Set Invalid Cellular Network Config",
		Method:                   "PUT",
		Url:                      fmt.Sprintf("%s/%s/configs/cellular", testUrlRoot, networkId),
		Payload:                  swaggerConfigString,
		Expected:                 `{"message":"Invalid config: validation failure list:\nvalidation failure list:\nmcc in body should match '^(\\d{3})$'"}`,
		Expect_http_error_status: true,
	}
	status, _, err = obsidian_test.RunTest(t, setConfigTestCase)
	assert.Equal(t, 400, status)

	// Fail swagger validation
	config.Epc.Mcc = "123"
	config.Ran.BandwidthMhz = 16
	protos.FillIn(config, swaggerConfig)
	marshaledCfg, err = swaggerConfig.MarshalBinary()
	assert.NoError(t, err)
	swaggerConfigString = string(marshaledCfg)

	setConfigTestCase = obsidian_test.Testcase{
		Name:                     "Set Invalid Cellular Network Config 2",
		Method:                   "PUT",
		Url:                      fmt.Sprintf("%s/%s/configs/cellular", testUrlRoot, networkId),
		Payload:                  swaggerConfigString,
		Expected:                 `{"message":"Invalid config: validation failure list:\nvalidation failure list:\nbandwidth_mhz in body should be one of [3 5 10 15 20]"}`,
		Expect_http_error_status: true,
	}
	status, _, err = obsidian_test.RunTest(t, setConfigTestCase)
	assert.Equal(t, 400, status)

	// Fail swagger validation
	config = test_utils.NewDefaultTDDNetworkConfig()
	swaggerConfig = &models.NetworkCellularConfigs{}
	protos.FillIn(config, swaggerConfig)
	swaggerConfig.Epc.NetworkServices = []string{"metering", "dpi", "bad"}
	marshaledCfg, err = swaggerConfig.MarshalBinary()
	assert.NoError(t, err)
	swaggerConfigString = string(marshaledCfg)

	setConfigTestCase = obsidian_test.Testcase{
		Name:                     "Set Invalid Network Service Name",
		Method:                   "PUT",
		Url:                      fmt.Sprintf("%s/%s/configs/cellular", testUrlRoot, networkId),
		Payload:                  swaggerConfigString,
		Expected:                 `{"message":"Invalid config: validation failure list:\nvalidation failure list:\nnetwork_services.2 in body should be one of [metering dpi policy_enforcement]"}`,
		Expect_http_error_status: true,
	}
	status, _, err = obsidian_test.RunTest(t, setConfigTestCase)
	assert.Equal(t, 400, status)
}

func TestSetBadOldConfigs(t *testing.T) {
	plugin.RegisterPluginForTests(t, &lteplugin.LteOrchestratorPlugin{})
	plugin.RegisterPluginForTests(t, &pluginimpl.BaseOrchestratorPlugin{})
	magmad_test_init.StartTestService(t)
	configurator_test_init.StartTestService(t)
	restPort := obsidian_test.StartObsidian(t)
	testUrlRoot := fmt.Sprintf("http://localhost:%d%s/networks", restPort, handlers.REST_ROOT)

	networkId := registerNetwork(t, "Test Network 1", "cellular_obsidian_test_network")

	// Fail RAN config check
	config := test_utils.OldTDDNetworkConfig()
	config.Ran.Earfcndl = 125000

	swaggerConfig := &models.NetworkCellularConfigs{}
	protos.FillIn(config, swaggerConfig)
	marshaledCfg, err := swaggerConfig.MarshalBinary()
	assert.NoError(t, err)
	swaggerConfigString := string(marshaledCfg)

	setConfigTestCase := obsidian_test.Testcase{
		Name:                     "Set Invalid Earcndl Config",
		Method:                   "POST",
		Url:                      fmt.Sprintf("%s/%s/configs/cellular", testUrlRoot, networkId),
		Payload:                  swaggerConfigString,
		Expected:                 `{"message":"Error converting config model: Invalid EARFCNDL: no matching band"}`,
		Expect_http_error_status: true,
	}
	status, _, err := obsidian_test.RunTest(t, setConfigTestCase)
	assert.Equal(t, 400, status)
}

func TestGetGatewayConfigs(t *testing.T) {
	plugin.RegisterPluginForTests(t, &lteplugin.LteOrchestratorPlugin{})
	plugin.RegisterPluginForTests(t, &pluginimpl.BaseOrchestratorPlugin{})
	magmad_test_init.StartTestService(t)
	configurator_test_init.StartTestService(t)
	restPort := obsidian_test.StartObsidian(t)
	testUrlRoot := fmt.Sprintf("http://localhost:%d%s/networks", restPort, handlers.REST_ROOT)

	networkId := registerNetwork(t, "Test Network 1", "cellular_obsidian_test_network")
	gatewayId := registerGateway(t, networkId, "g1")

	// Happy path
	expectedConfig := &models.GatewayCellularConfigs{}
	protos.FillIn(test_utils.NewDefaultGatewayConfig(), expectedConfig)
	marshaledCfg, err := expectedConfig.MarshalBinary()
	assert.NoError(t, err)
	expected := string(marshaledCfg)

	createConfigTestCase := obsidian_test.Testcase{
		Name:     "Create Cellular Gateway Config",
		Method:   "POST",
		Url:      fmt.Sprintf("%s/%s/gateways/%s/configs/cellular", testUrlRoot, networkId, gatewayId),
		Payload:  expected,
		Expected: `"g1"`,
	}
	obsidian_test.RunTest(t, createConfigTestCase)

	happyPathTestCase := obsidian_test.Testcase{
		Name:     "Get Cellular Gateway Config",
		Method:   "GET",
		Url:      fmt.Sprintf("%s/%s/gateways/%s/configs/cellular", testUrlRoot, networkId, gatewayId),
		Payload:  "",
		Expected: expected,
	}
	obsidian_test.RunTest(t, happyPathTestCase)

	// No good way to test invalid configs from datastore without dropping down
	// to raw magmad api/grpc or datastore fixtures, so let's skip that
	// for now
}

func TestSetGatewayConfigs(t *testing.T) {
	plugin.RegisterPluginForTests(t, &lteplugin.LteOrchestratorPlugin{})
	plugin.RegisterPluginForTests(t, &pluginimpl.BaseOrchestratorPlugin{})
	magmad_test_init.StartTestService(t)
	configurator_test_init.StartTestService(t)
	restPort := obsidian_test.StartObsidian(t)
	testUrlRoot := fmt.Sprintf("http://localhost:%d%s/networks", restPort, handlers.REST_ROOT)

	networkId := registerNetwork(t, "Test Network 1", "cellular_obsidian_test_network")
	gatewayId := registerGateway(t, networkId, "g2")

	// Happy path
	gatewayConfig := test_utils.NewDefaultGatewayConfig()
	swaggerConfig := &models.GatewayCellularConfigs{}
	protos.FillIn(gatewayConfig, swaggerConfig)
	marshaledCfg, err := swaggerConfig.MarshalBinary()
	assert.NoError(t, err)
	swaggerConfigString := string(marshaledCfg)

	createConfigTestCase := obsidian_test.Testcase{
		Name:     "Create Cellular Gateway Config",
		Method:   "POST",
		Url:      fmt.Sprintf("%s/%s/gateways/%s/configs/cellular", testUrlRoot, networkId, gatewayId),
		Payload:  swaggerConfigString,
		Expected: `"g2"`,
	}
	obsidian_test.RunTest(t, createConfigTestCase)

	gatewayConfig.Ran.Pci = 456
	gatewayConfig.Epc.IpBlock = "192.168.80.10/24" // Make sure filling handles IP properly
	gatewayConfig.NonEpsService.CsfbMcc = "123"
	gatewayConfig.NonEpsService.CsfbMnc = "23"
	gatewayConfig.NonEpsService.Lac = 10
	gatewayConfig.NonEpsService.CsfbRat = 1
	gatewayConfig.NonEpsService.Arfcn_2G = []int32{1, 2, 3}
	gatewayConfig.NonEpsService.NonEpsServiceControl = 2
	swaggerConfig = &models.GatewayCellularConfigs{}
	protos.FillIn(gatewayConfig, swaggerConfig)
	assert.Equal(t, gatewayConfig.Epc.IpBlock, swaggerConfig.Epc.IPBlock)

	marshaledCfg, err = swaggerConfig.MarshalBinary()
	assert.NoError(t, err)
	swaggerConfigString = string(marshaledCfg)

	setConfigTestCase := obsidian_test.Testcase{
		Name:     "Set Cellular Gateway Config",
		Method:   "PUT",
		Url:      fmt.Sprintf("%s/%s/gateways/%s/configs/cellular", testUrlRoot, networkId, gatewayId),
		Payload:  swaggerConfigString,
		Expected: "",
	}
	obsidian_test.RunTest(t, setConfigTestCase)
	getConfigTestCase := obsidian_test.Testcase{
		Name:     "Get Updated Cellular Gateway Config",
		Method:   "GET",
		Url:      fmt.Sprintf("%s/%s/gateways/%s/configs/cellular", testUrlRoot, networkId, gatewayId),
		Payload:  "",
		Expected: swaggerConfigString,
	}
	obsidian_test.RunTest(t, getConfigTestCase)

	// Fail proto validation (no swagger validation on gateway configs)
	gatewayConfig.Ran = nil
	swaggerConfig = &models.GatewayCellularConfigs{}
	protos.FillIn(gatewayConfig, swaggerConfig)
	marshaledCfg, err = swaggerConfig.MarshalBinary()
	assert.NoError(t, err)
	swaggerConfigString = string(marshaledCfg)

	setConfigTestCase = obsidian_test.Testcase{
		Name:                     "Set Invalid Cellular Gateway Config",
		Method:                   "PUT",
		Url:                      fmt.Sprintf("%s/%s/gateways/%s/configs/cellular", testUrlRoot, networkId, gatewayId),
		Payload:                  swaggerConfigString,
		Expected:                 `{"message":"Error converting config model: Gateway RAN config is nil"}`,
		Expect_http_error_status: true,
	}
	status, _, err := obsidian_test.RunTest(t, setConfigTestCase)
	assert.Equal(t, 400, status)

}

func testSetNetworkConfigs(t *testing.T, config *cellular_protos.CellularNetworkConfig, expectedConfig *cellular_protos.CellularNetworkConfig) {
	restPort := obsidian_test.StartObsidian(t)
	testUrlRoot := fmt.Sprintf("http://localhost:%d%s/networks", restPort, handlers.REST_ROOT)

	networkId := registerNetwork(t, "Test Network 1", "cellular_obsidian_test_network")

	// Happy path
	swaggerConfig := &models.NetworkCellularConfigs{}
	protos.FillIn(config, swaggerConfig)
	marshaledCfg, err := swaggerConfig.MarshalBinary()
	assert.NoError(t, err)
	swaggerConfigString := string(marshaledCfg)

	createConfigTestCase := obsidian_test.Testcase{
		Name:     "Create Cellular Network Config",
		Method:   "POST",
		Url:      fmt.Sprintf("%s/%s/configs/cellular", testUrlRoot, networkId),
		Payload:  swaggerConfigString,
		Expected: fmt.Sprintf(`"%s"`, networkId),
	}
	obsidian_test.RunTest(t, createConfigTestCase)

	config.Epc.Mcc = "123"
	config.Epc.SubProfiles = make(
		map[string]*cellular_protos.NetworkEPCConfig_SubscriptionProfile)
	config.Epc.SubProfiles["test"] =
		&cellular_protos.NetworkEPCConfig_SubscriptionProfile{
			MaxUlBitRate: 100, MaxDlBitRate: 200,
		}
	config.Ran.BandwidthMhz = 15

	expectedConfig.Epc.Mcc = "123"
	expectedConfig.Epc.SubProfiles = make(
		map[string]*cellular_protos.NetworkEPCConfig_SubscriptionProfile)
	expectedConfig.Epc.SubProfiles["test"] =
		&cellular_protos.NetworkEPCConfig_SubscriptionProfile{
			MaxUlBitRate: 100, MaxDlBitRate: 200,
		}
	expectedConfig.Ran.BandwidthMhz = 15

	swaggerConfig = &models.NetworkCellularConfigs{}
	protos.FillIn(config, swaggerConfig)
	swaggerConfig.Epc.NetworkServices = []string{"metering", "dpi", "policy_enforcement"}

	expectedSwaggerConfig := &models.NetworkCellularConfigs{}
	protos.FillIn(expectedConfig, expectedSwaggerConfig)
	expectedSwaggerConfig.Epc.NetworkServices = []string{"metering", "dpi", "policy_enforcement"}

	marshaledCfg, err = swaggerConfig.MarshalBinary()
	assert.NoError(t, err)
	swaggerConfigString = string(marshaledCfg)

	exMarshaledCfg, err := expectedSwaggerConfig.MarshalBinary()
	assert.NoError(t, err)
	exSwaggerConfigString := string(exMarshaledCfg)

	setConfigTestCase := obsidian_test.Testcase{
		Name:     "Set Cellular Network Config",
		Method:   "PUT",
		Url:      fmt.Sprintf("%s/%s/configs/cellular", testUrlRoot, networkId),
		Payload:  swaggerConfigString,
		Expected: "",
	}
	obsidian_test.RunTest(t, setConfigTestCase)
	getConfigTestCase := obsidian_test.Testcase{
		Name:     "Get Updated Cellular Network Config",
		Method:   "GET",
		Url:      fmt.Sprintf("%s/%s/configs/cellular", testUrlRoot, networkId),
		Payload:  "",
		Expected: exSwaggerConfigString,
	}
	obsidian_test.RunTest(t, getConfigTestCase)
}

func registerNetwork(t *testing.T, networkName string, networkId string) string {
	networkId, err := magmad.RegisterNetwork(
		&magmad_protos.MagmadNetworkRecord{Name: networkName},
		networkId)
	assert.NoError(t, err)
	return networkId
}

func registerGateway(t *testing.T, networkId string, gatewayId string) string {
	gatewayRecord := &magmad_protos.AccessGatewayRecord{
		HwId: &protos.AccessGatewayID{Id: gatewayId},
	}
	registeredId, err := magmad.RegisterGateway(networkId, gatewayRecord)
	assert.NoError(t, err)
	return registeredId
}
