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

package filter

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	k8stypes "k8s.io/apimachinery/pkg/types"
	backendmetrics "sigs.k8s.io/gateway-api-inference-extension/pkg/epp/backend/metrics"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/scheduling/types"
)

// Test for 1P1D PDFilter implementation
func TestPDFilterFunc(t *testing.T) {
	tests := []struct {
		name         string
		req          *types.LLMRequest
		input        []types.Pod
		output       []types.Pod
		outputHeader string
	}{
		{
			name:   "empty input",
			input:  []types.Pod{},
			output: []types.Pod{},
		},
		{
			name: "only prefill",
			input: []types.Pod{
				&types.PodMetrics{
					Metrics: &backendmetrics.Metrics{},
					Pod: &backendmetrics.Pod{
						NamespacedName: k8stypes.NamespacedName{Name: "pod1"},
						Address:        "1.2.3.4",
						Role:           backendmetrics.Prefill,
					},
				},
			},
			output:       []types.Pod{},
			outputHeader: "http://1.2.3.4:1234",
		},
		{
			name: "prefill and decode",
			input: []types.Pod{
				&types.PodMetrics{
					Metrics: &backendmetrics.Metrics{},
					Pod: &backendmetrics.Pod{
						NamespacedName: k8stypes.NamespacedName{Name: "pod1"},
						Address:        "1.2.3.4",
						Role:           backendmetrics.Prefill,
					},
				},
				&types.PodMetrics{
					Metrics: &backendmetrics.Metrics{},
					Pod: &backendmetrics.Pod{
						NamespacedName: k8stypes.NamespacedName{Name: "pod2"},
						Address:        "2.3.4.5",
						Role:           backendmetrics.Decode,
					},
				},
			},
			output: []types.Pod{
				&types.PodMetrics{
					Metrics: &backendmetrics.Metrics{},
					Pod: &backendmetrics.Pod{
						NamespacedName: k8stypes.NamespacedName{Name: "pod2"},
						Address:        "2.3.4.5",
						Role:           backendmetrics.Decode,
					},
				},
			},
			outputHeader: "http://1.2.3.4:1234",
		},
		{
			name: "prefill and both",
			input: []types.Pod{
				&types.PodMetrics{
					Metrics: &backendmetrics.Metrics{},
					Pod: &backendmetrics.Pod{
						NamespacedName: k8stypes.NamespacedName{Name: "pod1"},
						Address:        "1.2.3.4",
						Role:           backendmetrics.Prefill,
					},
				},
				&types.PodMetrics{
					Metrics: &backendmetrics.Metrics{},
					Pod: &backendmetrics.Pod{
						NamespacedName: k8stypes.NamespacedName{Name: "pod2"},
						Address:        "2.3.4.5",
						Role:           backendmetrics.Both,
					},
				},
			},
			output: []types.Pod{
				&types.PodMetrics{
					Metrics: &backendmetrics.Metrics{},
					Pod: &backendmetrics.Pod{
						NamespacedName: k8stypes.NamespacedName{Name: "pod2"},
						Address:        "2.3.4.5",
						Role:           backendmetrics.Both,
					},
				},
			},
			outputHeader: "http://1.2.3.4:1234",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := types.NewSchedulingContext(context.Background(), test.req, test.input, 1234)
			got := prefillDecodeFilterFunc(ctx, test.input)

			if diff := cmp.Diff(test.output, got); diff != "" {
				t.Errorf("Unexpected output (-want +got): %v", diff)
			}

			prefillHeader, ok := ctx.MutatedHeaders[prefillPodHeader]

			if !ok && len(test.outputHeader) > 0 {
				t.Errorf("Missing prefill header")
			}

			if prefillHeader != test.outputHeader {
				t.Errorf("Invalid prefill header want <%s>,  got <%s>", test.outputHeader, prefillHeader)
			}
		})
	}
}
