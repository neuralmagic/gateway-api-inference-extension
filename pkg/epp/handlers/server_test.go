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

package handlers

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	pb "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stypes "k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/gateway-api-inference-extension/api/v1alpha2"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/backend/metrics"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/datastore"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/scheduling/types"
	testutil "sigs.k8s.io/gateway-api-inference-extension/pkg/epp/util/testing"
)

const (
	bufSize                    = 1024 * 1024
	podName                    = "pod1"
	podAddress                 = "1.2.3.4"
	poolPort                   = int32(5678)
	destinationEndpointHintKey = "test-target"
)

var testListener *bufconn.Listener

func TestServer(t *testing.T) {
	theHeaderValue := "body"
	requestHeader := "x-test"

	expectedRequestHeaders := map[string]string{
		destinationEndpointHintKey: fmt.Sprintf("%s:%d", podAddress, poolPort), "x-test2": "123", "x-test3": "hello",
		"Content-Length": "51"}
	expectedResponseHeaders := map[string]string{"x-went-into-resp-headers": "true", "x-test2": "123", "x-test3": "hello"}
	expectedSchedulerHeaders := map[string]string{":method": "POST", requestHeader: theHeaderValue}

	t.Run("server", func(t *testing.T) {
		scheduler := &testScheduler{}
		ctx, cancel, errChan := setup(t, scheduler)
		process, conn := getProcessClient(ctx, t)
		defer conn.Close()

		// Send request headers - no response expected
		request := &pb.ProcessingRequest{
			Request: &pb.ProcessingRequest_RequestHeaders{
				RequestHeaders: &pb.HttpHeaders{
					Headers: &corev3.HeaderMap{
						Headers: []*corev3.HeaderValue{
							{Key: requestHeader, RawValue: []byte(theHeaderValue)},
							{Key: ":method", RawValue: []byte("POST")},
						},
					},
				},
			},
		}
		err := process.Send(request)
		if err != nil {
			t.Error("Error sending request headers", err)
		}

		// Send request body
		requestBody := "{\"model\":\"food-review\",\"prompt\":\"Is banana tasty?\"}"
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
				checkHeaders(t, responseReqHeaders.GetRequestHeaders().Response, expectedRequestHeaders)
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
				if string(body) != requestBody {
					t.Errorf("Incorrect body %s expected %s", string(body), requestBody)
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
		request = &pb.ProcessingRequest{
			Request: &pb.ProcessingRequest_ResponseHeaders{
				ResponseHeaders: &pb.HttpHeaders{
					Headers: &corev3.HeaderMap{
						Headers: []*corev3.HeaderValue{
							{Key: requestHeader, Value: theHeaderValue},
							{Key: ":method", Value: "POST"},
						},
					},
				},
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
				checkHeaders(t, response.GetResponseHeaders().Response, expectedResponseHeaders)
			}
		}

		cancel()
		<-errChan
		testListener.Close()
	})

}

func setup(t *testing.T, scheduler Scheduler) (ctx context.Context, cancel context.CancelFunc, errChan chan error) {
	testListener = bufconn.Listen(bufSize)

	logger := klog.Background()
	ctx = klog.NewContext(context.Background(), logger)
	ctx, cancel = context.WithCancel(ctx)

	pmf := metrics.NewPodMetricsFactory(&metrics.FakePodMetricsClient{}, time.Minute)
	ds := datastore.NewDatastore(ctx, pmf)

	tsModel := "food-review"
	model1ts := testutil.MakeInferenceModel("model1").
		CreationTimestamp(metav1.Unix(1000, 0)).
		ModelName(tsModel).ObjRef()
	ds.ModelSetIfOlder(model1ts)

	ds.PodUpdateOrAddIfNotExist(&v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: podName},
	})

	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = v1alpha2.Install(scheme)
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(model1ts).
		Build()
	pool := testutil.MakeInferencePool("test-pool1").Namespace("ns1").ObjRef()
	pool.Spec.TargetPortNumber = poolPort
	_ = ds.PoolSet(context.Background(), fakeClient, pool)

	streamingServer := NewStreamingServer(scheduler, "", destinationEndpointHintKey, ds)

	errChan = make(chan error)
	go func() {
		err := launch(streamingServer, ctx, testListener)
		if err != nil {
			t.Error("Error launching listener", err)
		}
		errChan <- err
	}()

	time.Sleep(2 * time.Second)
	return
}

func testDialer(context.Context, string) (net.Conn, error) {
	return testListener.Dial()
}

func getProcessClient(ctx context.Context, t *testing.T) (pb.ExternalProcessor_ProcessClient, *grpc.ClientConn) {
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(testDialer),
	}
	conn, err := grpc.NewClient("passthrough://bufconn", opts...)
	if err != nil {
		t.Error(err)
		return nil, nil
	}

	extProcClient := pb.NewExternalProcessorClient(conn)
	process, err := extProcClient.Process(ctx)
	if err != nil {
		t.Error(err)
		return nil, nil
	}

	return process, conn
}

// launch actually starts the server (enables testing)
func launch(s *StreamingServer, ctx context.Context, listener net.Listener) error {
	grpcServer := grpc.NewServer()

	pb.RegisterExternalProcessorServer(grpcServer, s)

	// Shutdown on context closed.
	// Terminate the server on context closed.
	go func() {
		<-ctx.Done()
		grpcServer.GracefulStop()
	}()

	if err := grpcServer.Serve(listener); err != nil {
		return err
	}

	return nil
}

func checkHeaders(t *testing.T, response *pb.CommonResponse, expectedHeaders map[string]string) {
	headers := response.HeaderMutation.SetHeaders
	for expectedKey, expectedValue := range expectedHeaders {
		found := false
		for _, header := range headers {
			if header.Header.Key == expectedKey {
				if expectedValue != string(header.Header.RawValue) {
					t.Errorf("Incorrect value for header %s, want %s got %s", expectedKey, expectedValue,
						string(header.Header.RawValue))
				}
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Missing header %s", expectedKey)
		}
	}

	for _, header := range headers {
		expectedValue, ok := expectedHeaders[header.Header.Key]
		if !ok {
			t.Errorf("Unexpected header %s", header.Header.Key)
		} else if expectedValue != string(header.Header.RawValue) {
			t.Errorf("Incorrect value for header %s, want %s got %s", header.Header.Key, expectedValue,
				string(header.Header.RawValue))
		}
	}
}

type testScheduler struct {
	requestHeaders map[string]string
}

func (ts *testScheduler) RunPostResponsePlugins(ctx context.Context, req *types.LLMRequest, tragetPodName string) (*types.Result, error) {
	return &types.Result{
		MutatedHeaders: map[string]string{"x-test2": "123", "x-test3": "hello"},
	}, nil
}

func (ts *testScheduler) Schedule(ctx context.Context, req *types.LLMRequest) (*types.Result, error) {
	ts.requestHeaders = req.Headers

	return &types.Result{
		TargetPod: &types.PodMetrics{
			Pod: &metrics.Pod{NamespacedName: k8stypes.NamespacedName{Name: podName}, Address: podAddress},
			Metrics: &metrics.Metrics{
				MaxActiveModels: 2,
			},
		},
		MutatedHeaders: map[string]string{"x-test2": "123", "x-test3": "hello"},
	}, nil
}
