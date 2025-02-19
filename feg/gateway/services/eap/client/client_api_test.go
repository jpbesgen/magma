/*
Copyright (c) Facebook, Inc. and its affiliates.
All rights reserved.

This source code is licensed under the BSD-style license found in the
LICENSE file in the root directory of this source tree.
*/
package client_test

import (
	"reflect"
	"testing"
	"time"

	cp "magma/feg/cloud/go/protos"
	"magma/feg/cloud/go/protos/mconfig"
	"magma/feg/gateway/registry"
	"magma/feg/gateway/services/aaa/protos"
	"magma/feg/gateway/services/eap"
	"magma/feg/gateway/services/eap/client"
	eapp "magma/feg/gateway/services/eap/protos"
	"magma/feg/gateway/services/eap/providers/aka"
	"magma/feg/gateway/services/eap/providers/aka/servicers"
	_ "magma/feg/gateway/services/eap/providers/aka/servicers/handlers"
	eap_test "magma/feg/gateway/services/eap/test"
	"magma/orc8r/cloud/go/test_utils"
)

var (
	expectedMppeRecvKey = []byte(
		"\x95\x63\x3c\x3a\xa5\x8b\x48\xbe\xde\x6d\x2c\x1a\x91\x70\x71\xf5" +
			"\x63\xd4\xed\x7f\xba\xb3\xec\x61\xed\x7e\x3a\xf4\x82\x06\x58\x71" +
			"\x8c\xf7\xee\x86\x81\x0d\xf4\xf9\xf4\xb7\xb9\xdd\x14\xca\xc3\xbd\x95\x80")
	expectedMppeRecvKeySalt = []byte("\x95\x63")

	expectedMppeSendKey = []byte(
		"\x9b\x87\x83\x49\x6a\x78\xcc\xaa\x34\x4e\x45\x51\x7f\x15\x37\xf9" +
			"\x30\x94\x26\x07\x60\x68\x97\xf0\xb5\x69\xab\x1d\x61\x9d\x8b\xa9" +
			"\x85\x3c\xc8\xaf\x68\x4b\xaa\x8f\x8f\x77\x5f\x68\x94\xf0\xcd\xc6\xc9\x2f")
	expectedMppeSendKeySalt = []byte("\x9b\x87")

	authenticator = []byte{
		0x9f, 0xe8, 0xff, 0xcb, 0xc9, 0xd4, 0x85, 0x97, 0xb9, 0x5b, 0x79, 0x7c, 0x2d, 0xf5, 0x43, 0x31,
	}
	sharedSecret = []byte("1qaz2wsx")
	msisdn       = "123456789"
)

type testEapClient struct{}

func (c testEapClient) Handle(in *protos.Eap) (*protos.Eap, error) {
	return client.Handle(in)
}

func TestEAPClientApi(t *testing.T) {
	srv, lis := test_utils.NewTestService(t, registry.ModuleName, registry.SWX_PROXY)
	var service eap_test.SwxProxy
	cp.RegisterSwxProxyServer(srv.GrpcServer, service)
	go srv.RunTest(lis)

	eapSrv, eapLis := test_utils.NewTestService(t, registry.ModuleName, registry.EAP_AKA)
	servicer, err := servicers.NewEapAkaService(nil)
	if err != nil {
		t.Fatalf("failed to create EAP AKA Service: %v", err)
		return
	}
	eapp.RegisterEapServiceServer(eapSrv.GrpcServer, servicer)
	go eapSrv.RunTest(eapLis)

	go eap_test.Auth(t, testEapClient{}, eap_test.IMSI2, 10, nil) // start IMSI2 tests in parallel

	tst := eap_test.Units[eap_test.IMSI1]
	eapCtx := &protos.Context{SessionId: eap.CreateSessionId()}
	peap, err := client.Handle(&protos.Eap{Payload: tst.EapIdentityResp, Ctx: eapCtx})
	if err != nil {
		t.Fatalf("Error Handling Test EAP: %v", err)
	}
	if !reflect.DeepEqual([]byte(peap.GetPayload()), tst.ExpectedChallengeReq) {
		t.Fatalf(
			"Unexpected identityResponse EAP\n\tReceived: %.3v\n\tExpected: %.3v",
			peap.GetPayload(), tst.ExpectedChallengeReq)
	}

	servicer.SetSessionAuthenticatedTimeout(time.Millisecond * 200)

	eapCtx = peap.GetCtx()
	peap, err = client.Handle(&protos.Eap{Payload: tst.EapChallengeResp, Ctx: eapCtx})
	if err != nil {
		t.Fatalf("Error Handling Test Challenge EAP: %v", err)
	}
	if !reflect.DeepEqual([]byte(peap.GetPayload()), []byte(eap_test.SuccessEAP)) {
		t.Fatalf(
			"Unexpected Challenge Response EAP\n\tReceived: %.3v\n\tExpected: %.3v",
			peap.GetPayload(), []byte(eap_test.SuccessEAP))
	}
	// Check that we got expected MSISDN with the success EAP
	if peap.GetCtx().Msisdn != tst.MSISDN {
		t.Fatalf("Unexpected MSISDN: %s, expected: %s", eapCtx.Msisdn, tst.MSISDN)
	}

	// We should get a valid MSR within the auth success EAP Ctx, verify that we generated valid
	// MS-MPPE-Recv-Key & MS-MPPE-Send-Key according to https://tools.ietf.org/html/rfc2548
	genMS_MPPE_Recv_Key := append(
		expectedMppeRecvKeySalt,
		eap.EncodeMsMppeKey(expectedMppeRecvKeySalt, peap.GetCtx().Msk[0:32], authenticator, sharedSecret)...)

	genMS_MPPE_Send_Key := append(
		expectedMppeSendKeySalt,
		eap.EncodeMsMppeKey(expectedMppeSendKeySalt, peap.GetCtx().Msk[32:], authenticator, sharedSecret)...)

	if !reflect.DeepEqual(genMS_MPPE_Recv_Key, expectedMppeRecvKey) {
		t.Fatalf(
			"MS_MPPE_Recv_Keys mismatch.\n\tGenerated MS_MPPE_Recv_Key(%d): %v\n\tExpected  MS_MPPE_Recv_Key(%d): %v",
			len(genMS_MPPE_Recv_Key), genMS_MPPE_Recv_Key, len(expectedMppeRecvKey), expectedMppeRecvKey)
	}
	if !reflect.DeepEqual(genMS_MPPE_Send_Key, expectedMppeSendKey) {
		t.Fatalf(
			"MS_MPPE_Send_Keys mismatch.\n\tGenerated MS_MPPE_Send_Key(%d): %v\n\tExpected  MS_MPPE_Send_Key(%d): %v",
			len(genMS_MPPE_Send_Key), genMS_MPPE_Send_Key, len(expectedMppeSendKey), expectedMppeSendKey)
	}

	time.Sleep(time.Millisecond * 10)

	eapCtx = peap.GetCtx()
	peap, err = client.Handle(&protos.Eap{Payload: tst.EapChallengeResp, Ctx: eapCtx})
	if err != nil {
		t.Fatalf("Error Handling Second Test Challenge EAP within Auth timeout window: %v", err)
	}
	if !reflect.DeepEqual([]byte(peap.GetPayload()), []byte(eap_test.SuccessEAP)) {
		t.Fatalf(
			"Unexpected Challenge Response EAP\n\tReceived: %.3v\n\tExpected: %.3v",
			peap.GetPayload(), []byte(eap_test.SuccessEAP))
	}

	time.Sleep(servicer.SessionAuthenticatedTimeout() + time.Millisecond*10)

	eapCtx = peap.GetCtx()
	peap, err = client.Handle(&protos.Eap{Payload: tst.EapChallengeResp, Ctx: eapCtx})
	if err != nil {
		t.Fatalf("Unexpected Error for removed Session ID: %s - %v", eapCtx.SessionId, err)
	}
	notifAkaEap := aka.NewAKANotificationReq(eap.Packet(tst.EapChallengeResp).Identifier(), aka.NOTIFICATION_FAILURE)
	if !reflect.DeepEqual(peap.GetPayload(), []byte(notifAkaEap)) {
		t.Fatalf(
			"Unexpected Challenge Response for removed Session\n\tReceived: %.3v\n\tExpected: %.3v",
			peap.GetPayload(), notifAkaEap)
	}

	// Test timeout
	servicer.SetChallengeTimeout(time.Millisecond * 100)
	eapCtx = &protos.Context{SessionId: eap.CreateSessionId()}
	peap, err = client.Handle(&protos.Eap{Payload: tst.EapIdentityResp, Ctx: eapCtx})
	if err != nil {
		t.Fatalf("Error Handling second Test EAP: %v", err)
	}
	time.Sleep(servicer.ChallengeTimeout() + time.Millisecond*20)

	eapCtx = peap.GetCtx()
	peap, err = client.Handle(&protos.Eap{Payload: tst.EapChallengeResp, Ctx: eapCtx})
	if err != nil {
		t.Fatalf("Unxpected Error for timed out Session ID: %s - %v", eapCtx.SessionId, err)
	}
	notifAkaEap = aka.NewAKANotificationReq(eap.Packet(tst.EapChallengeResp).Identifier(), aka.NOTIFICATION_FAILURE)
	if !reflect.DeepEqual(peap.GetPayload(), []byte(notifAkaEap)) {
		t.Fatalf(
			"Unexpected Challenge Response for timed out Session\n\tReceived: %.3v\n\tExpected: %.3v",
			peap.GetPayload(), notifAkaEap)
	}
}

func TestEAPClientApiConcurent(t *testing.T) {
	srv, lis := test_utils.NewTestService(t, registry.ModuleName, registry.SWX_PROXY)
	var service eap_test.SwxProxy
	cp.RegisterSwxProxyServer(srv.GrpcServer, service)
	go srv.RunTest(lis)

	eapSrv, eapLis := test_utils.NewTestService(t, registry.ModuleName, registry.EAP_AKA)
	servicer, err := servicers.NewEapAkaService(&mconfig.EapAkaConfig{
		Timeout: &mconfig.EapAkaConfig_Timeouts{
			ChallengeMs:            300,
			ErrorNotificationMs:    200,
			SessionMs:              500,
			SessionAuthenticatedMs: 1000,
		}})
	if err != nil {
		t.Fatalf("failed to create EAP AKA Service: %v", err)
		return
	}
	eapp.RegisterEapServiceServer(eapSrv.GrpcServer, servicer)
	go eapSrv.RunTest(eapLis)

	done := make(chan error)
	go eap_test.Auth(t, testEapClient{}, eap_test.IMSI1, 99, done)
	go eap_test.Auth(t, testEapClient{}, eap_test.IMSI2, 88, done)
	eap_test.Auth(t, testEapClient{}, eap_test.IMSI1, 77, nil)
	<-done
	<-done // wait for test 1 & 2 to complete
}
