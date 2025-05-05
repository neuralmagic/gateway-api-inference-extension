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

package scheduling

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	k8stypes "k8s.io/apimachinery/pkg/types"
	backendmetrics "sigs.k8s.io/gateway-api-inference-extension/pkg/epp/backend/metrics" // Import config for thresholds
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/scheduling/plugins"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/scheduling/plugins/filter"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/scheduling/types"
)

// Tests the default scheduler configuration and expected behavior.
func TestPDSchedule(t *testing.T) {
	// Set configuration
	PDEnabled = true
	promptLengthThreshold = 10
	prefillConfig.filters = []plugins.Filter{filter.PrefillFilter}
	prefillConfig.scorers = map[plugins.Scorer]int{}
	decodeConfig.filters = []plugins.Filter{filter.DecodeFilter}
	decodeConfig.scorers = map[plugins.Scorer]int{}

	pod1 := &backendmetrics.FakePodMetrics{
		Pod: &backendmetrics.Pod{
			NamespacedName: k8stypes.NamespacedName{Name: "pod1"},
			Address:        "1.2.3.4",
			Role:           backendmetrics.Prefill,
		},
		Metrics: &backendmetrics.Metrics{},
	}
	pod2 := &backendmetrics.FakePodMetrics{
		Pod: &backendmetrics.Pod{
			NamespacedName: k8stypes.NamespacedName{Name: "pod2"},
			Address:        "5.6.7.8",
			Role:           backendmetrics.Decode,
		},
		Metrics: &backendmetrics.Metrics{},
	}
	wantPod1 := &types.PodMetrics{
		Pod: &backendmetrics.Pod{
			NamespacedName: k8stypes.NamespacedName{Name: "pod1"},
			Address:        "1.2.3.4",
			Role:           backendmetrics.Prefill,
		},
		Metrics: &backendmetrics.Metrics{
			ActiveModels:  map[string]int{},
			WaitingModels: map[string]int{},
		},
	}
	wantPod2 := &types.PodMetrics{
		Pod: &backendmetrics.Pod{
			NamespacedName: k8stypes.NamespacedName{Name: "pod2"},
			Address:        "5.6.7.8",
			Role:           backendmetrics.Decode,
		},
		Metrics: &backendmetrics.Metrics{
			ActiveModels:  map[string]int{},
			WaitingModels: map[string]int{},
		},
	}

	tests := []struct {
		name    string
		req     *types.LLMRequest
		input   []*backendmetrics.FakePodMetrics
		wantRes *types.Result
		err     bool
	}{
		{
			name: "no pods in datastore",
			req: &types.LLMRequest{
				Model:               "any-model",
				ResolvedTargetModel: "any-model",
				Critical:            true,
				Prompt:              "12345678901",
			},
			input: []*backendmetrics.FakePodMetrics{},
			err:   true,
		},
		{
			name: "one pod, short prompt",
			req: &types.LLMRequest{
				Model:               "critical",
				ResolvedTargetModel: "critical",
				Critical:            true,
				Prompt:              "123",
			},
			// pod1 will be picked because it is the only one pod
			input: []*backendmetrics.FakePodMetrics{pod1},
			wantRes: &types.Result{
				TargetPod: &types.ScoredPod{
					Pod: wantPod1,
				},
				MutatedHeaders: map[string]string{},
			},
		},
		{
			name: "1P1D",
			req: &types.LLMRequest{
				Model:               "critical",
				ResolvedTargetModel: "critical",
				Critical:            true,
				Prompt:              "12345678901",
			},
			// pod2 will be picked because it is the decode pod
			input: []*backendmetrics.FakePodMetrics{pod1, pod2},
			wantRes: &types.Result{
				TargetPod: &types.ScoredPod{
					Pod:   wantPod2,
					Score: 0.0,
				},
				MutatedHeaders: map[string]string{"x-prefiller-url": "http://1.2.3.4:0"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scheduler := NewPDScheduler(&fakeDataStore{pods: test.input})
			got, err := scheduler.Schedule(context.Background(), test.req)

			fmt.Printf("Test %s:\n", test.name)
			fmt.Printf("Result: %#v\n", got)
			fmt.Printf("Expected: %#v\n", test.wantRes)

			if test.err != (err != nil) {
				t.Errorf("Unexpected error, got %v, want %v", err, test.err)
			}

			if diff := cmp.Diff(test.wantRes, got); diff != "" {
				t.Errorf("Unexpected output (-want +got): %v", diff)
			}
		})
	}
}
