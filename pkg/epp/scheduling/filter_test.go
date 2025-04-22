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
	"errors"
	"testing"

	"github.com/google/go-cmp/cmp"
	backendmetrics "sigs.k8s.io/gateway-api-inference-extension/pkg/epp/backend/metrics"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/scheduling/types"
)

func TestFilter(t *testing.T) {
	tests := []struct {
		name   string
		req    *types.LLMRequest
		input  []*types.PodMetrics
		output []*types.PodMetrics
		err    bool
		filter *decisionTreeFilter
	}{
		{
			name: "simple filter without successor, failure",
			filter: &decisionTreeFilter{
				current: &basicFilter{
					name: "error",
					filter: func(ctx *types.Context, pods []*types.PodMetrics) ([]*types.PodMetrics, error) {
						return nil, errors.New("filter error")
					},
				},
			},
			err: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := types.NewContext(context.Background(), test.req, test.input)
			got, err := test.filter.Filter(ctx, test.input)
			if test.err != (err != nil) {
				t.Errorf("Unexpected error, got %v, want %v", err, test.err)
			}

			if diff := cmp.Diff(test.output, got); diff != "" {
				t.Errorf("Unexpected output (-want +got): %v", diff)
			}
		})
	}
}

func TestFilterFunc(t *testing.T) {
	tests := []struct {
		name   string
		f      filterFunc
		req    *types.LLMRequest
		input  []*types.PodMetrics
		output []*types.PodMetrics
		err    bool
	}{
		{
			name:   "least queuing empty input",
			f:      leastQueuingFilterFunc,
			input:  []*types.PodMetrics{},
			output: []*types.PodMetrics{},
		},
		{
			name: "least queuing",
			f:    leastQueuingFilterFunc,
			input: []*types.PodMetrics{
				{
					Metrics: &backendmetrics.Metrics{
						WaitingQueueSize: 0,
					},
				},
				{
					Metrics: &backendmetrics.Metrics{
						WaitingQueueSize: 3,
					},
				},
				{
					Metrics: &backendmetrics.Metrics{
						WaitingQueueSize: 10,
					},
				},
			},
			output: []*types.PodMetrics{
				{
					Metrics: &backendmetrics.Metrics{
						WaitingQueueSize: 0,
					},
				},
				{
					Metrics: &backendmetrics.Metrics{
						WaitingQueueSize: 3,
					},
				},
			},
		},
		{
			name:   "least kv cache empty input",
			f:      leastKVCacheFilterFunc,
			input:  []*types.PodMetrics{},
			output: []*types.PodMetrics{},
		},
		{
			name: "least kv cache",
			f:    leastKVCacheFilterFunc,
			input: []*types.PodMetrics{
				{
					Metrics: &backendmetrics.Metrics{
						KVCacheUsagePercent: 0,
					},
				},
				{
					Metrics: &backendmetrics.Metrics{
						KVCacheUsagePercent: 0.3,
					},
				},
				{
					Metrics: &backendmetrics.Metrics{
						KVCacheUsagePercent: 1.0,
					},
				},
			},
			output: []*types.PodMetrics{
				{
					Metrics: &backendmetrics.Metrics{
						KVCacheUsagePercent: 0,
					},
				},
				{
					Metrics: &backendmetrics.Metrics{
						KVCacheUsagePercent: 0.3,
					},
				},
			},
		},
		{
			name: "lowQueueAndLessThanKVCacheThresholdPredicate",
			f:    toFilterFunc(queueThresholdPredicate(0).and(kvCacheThresholdPredicate(0.8))),
			input: []*types.PodMetrics{
				{
					// This pod should be returned.
					Metrics: &backendmetrics.Metrics{
						WaitingQueueSize:    0,
						KVCacheUsagePercent: 0,
					},
				},
				{
					// Queue is non zero, despite low kv cache, should not return.
					Metrics: &backendmetrics.Metrics{
						WaitingQueueSize:    1,
						KVCacheUsagePercent: 0.3,
					},
				},
				{
					// High kv cache despite zero queue, should not return
					Metrics: &backendmetrics.Metrics{
						WaitingQueueSize:    0,
						KVCacheUsagePercent: 1.0,
					},
				},
			},
			output: []*types.PodMetrics{
				{
					Metrics: &backendmetrics.Metrics{
						WaitingQueueSize:    0,
						KVCacheUsagePercent: 0,
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := types.NewContext(context.Background(), test.req, test.input)
			got, err := test.f(ctx, test.input)
			if test.err != (err != nil) {
				t.Errorf("Unexpected error, got %v, want %v", err, test.err)
			}

			if diff := cmp.Diff(test.output, got); diff != "" {
				t.Errorf("Unexpected output (-want +got): %v", diff)
			}
		})
	}
}
