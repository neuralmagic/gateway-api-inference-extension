/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package server

import (
	"context"
	"fmt"
	"testing"

	pb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/backend"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/backend/metrics"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/handlers"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/requestcontrol"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/scheduling/types"
	"sigs.k8s.io/gateway-api-inference-extension/test/utils"
)

const (
	bufSize                    = 1024 * 1024
	podName                    = "pod1"
	podAddress                 = "1.2.3.4"
	poolPort                   = int32(5678)
	destinationEndpointHintKey = "test-target"
	namespace                  = "ns1"
)

func TestServer(t *testing.T) {
	theHeaderValue := "body"
	requestHeader := "x-test"

	expectedRequestHeaders := map[string]string{":method": "POST", requestHeader: theHeaderValue,
		destinationEndpointHintKey: fmt.Sprintf("%s:%d", podAddress, poolPort), "Content-Length": "42"}
	expectedResponseHeaders := map[string]string{"x-went-into-resp-headers": "true"}
	expectedSchedulerHeaders := map[string]string{":method": "POST", requestHeader: theHeaderValue}

	t.Run("server", func(t *testing.T) {
		tsModel := "food-review"
		model := &v1alpha2.InferenceModel{
			Spec: v1alpha2.InferenceModelSpec{
				TargetModels: []v1alpha2.TargetModel{
					{
						Name: "v1",
					},
				},
				ModelName: tsModel,
			},
			ObjectMeta: metav1.ObjectMeta{
				CreationTimestamp: metav1.Unix(1000, 0),
			},
		}

		scheduler := &testScheduler{}
		ctx, cancel, ds, _ := utils.PrepareForTestStreamingServer([]*v1alpha2.InferenceModel{model},
			[]*v1.Pod{{ObjectMeta: metav1.ObjectMeta{Name: podName}}}, "test-pool1", namespace, poolPort)

		streamingServer := handlers.NewStreamingServer(namespace, destinationEndpointHintKey, ds, requestcontrol.NewDirector(ds, scheduler))

		testListener, errChan := utils.SetupTestStreamingServer(t, ctx, ds, streamingServer)
		process, conn := utils.GetStreamingServerClient(ctx, t)
		defer conn.Close()

		// Send request headers - no response expected
		headers := utils.BuildEnvoyGRPCHeaders(map[string]string{requestHeader: theHeaderValue, ":method": "POST"}, true)
		request := &pb.ProcessingRequest{
			Request: &pb.ProcessingRequest_RequestHeaders{
				RequestHeaders: headers,
			},
		}
		err := process.Send(request)
		if err != nil {
			t.Error("Error sending request headers", err)
		}

		// Send request body
		requestBody := "{\"model\":\"food-review\",\"prompt\":\"Is banana tasty?\"}"
		expectedBody := "{\"model\":\"v1\",\"prompt\":\"Is banana tasty?\"}"
		request = &pb.ProcessingRequest{
			Request: &pb.ProcessingRequest_RequestBody{
				RequestBody: &pb.HttpBody{
					Body:        []byte(requestBody),
					EndOfStream: true,
				},
			},
		}
		err = process.Send(request)
		if err != nil {
			t.Error("Error sending request body", err)
		}

		// Receive request headers and check
		responseReqHeaders, err := process.Recv()
		if err != nil {
			t.Error("Error receiving response", err)
		} else {
			if responseReqHeaders == nil || responseReqHeaders.GetRequestHeaders() == nil ||
				responseReqHeaders.GetRequestHeaders().Response == nil ||
				responseReqHeaders.GetRequestHeaders().Response.HeaderMutation == nil ||
				responseReqHeaders.GetRequestHeaders().Response.HeaderMutation.SetHeaders == nil {
				t.Error("Invalid request headers response")
			} else {
				if !utils.CheckEnvoyGRPCHeaders(t, responseReqHeaders.GetRequestHeaders().Response, expectedRequestHeaders) {
					t.Error("Incorrect request headers")
				}
			}
		}

		// Receive request body and check
		responseReqBody, err := process.Recv()
		if err != nil {
			t.Error("Error receiving response", err)
		} else {
			if responseReqBody == nil || responseReqBody.GetRequestBody() == nil ||
				responseReqBody.GetRequestBody().Response == nil ||
				responseReqBody.GetRequestBody().Response.BodyMutation == nil ||
				responseReqBody.GetRequestBody().Response.BodyMutation.GetStreamedResponse() == nil {
				t.Error("Invalid request body response")
			} else {
				body := responseReqBody.GetRequestBody().Response.BodyMutation.GetStreamedResponse().Body
				if string(body) != expectedBody {
					t.Errorf("Incorrect body %s expected %s", string(body), expectedBody)
				}
			}
		}

		// Check headers passed to the scheduler
		if len(scheduler.requestHeaders) != 2 {
			t.Errorf("Incorrect number of request headers %d instead of 2", len(scheduler.requestHeaders))
		}
		for expectedKey, expectedValue := range expectedSchedulerHeaders {
			got, ok := scheduler.requestHeaders[expectedKey]
			if !ok {
				t.Errorf("Missing header %s", expectedKey)
			} else if got != expectedValue {
				t.Errorf("Incorrect value for header %s, want %s got %s", expectedKey, expectedValue, got)
			}
		}

		// Send response headers
		headers = utils.BuildEnvoyGRPCHeaders(map[string]string{requestHeader: theHeaderValue, ":method": "POST"}, false)
		request = &pb.ProcessingRequest{
			Request: &pb.ProcessingRequest_ResponseHeaders{
				ResponseHeaders: headers,
			},
		}
		err = process.Send(request)
		if err != nil {
			t.Error("Error sending response", err)
		}

		// Receive response headers and check
		response, err := process.Recv()
		if err != nil {
			t.Error("Error receiving response", err)
		} else {
			if response == nil || response.GetResponseHeaders() == nil || response.GetResponseHeaders().Response == nil ||
				response.GetResponseHeaders().Response.HeaderMutation == nil ||
				response.GetResponseHeaders().Response.HeaderMutation.SetHeaders == nil {
				t.Error("Invalid response")
			} else {
				if !utils.CheckEnvoyGRPCHeaders(t, response.GetResponseHeaders().Response, expectedResponseHeaders) {
					t.Error("Incorrect response headers")
				}
			}
		}

		cancel()
		<-errChan
		testListener.Close()
	})
}

type testScheduler struct {
	requestHeaders map[string]string
}

func (ts *testScheduler) Schedule(ctx context.Context, req *types.LLMRequest) (*types.Result, error) {
	ts.requestHeaders = req.Headers

	return &types.Result{
		TargetPod: &types.PodMetrics{
			Pod: &backend.Pod{NamespacedName: k8stypes.NamespacedName{Name: podName}, Address: podAddress},
			Metrics: &metrics.Metrics{
				MaxActiveModels: 2,
			},
		},
	}, nil
}
