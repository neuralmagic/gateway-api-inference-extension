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
	"testing"
	"time"

	k8stypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
	backendmetrics "sigs.k8s.io/gateway-api-inference-extension/pkg/epp/backend/metrics"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/scheduling/types"
)

func TestPrefixAwareScorer(t *testing.T) {
	ctx := context.Background()
	logger := log.FromContext(ctx)
	ctx = log.IntoContext(ctx, logger)

	// Create a prefix store with test configuration
	prefixStore := NewPrefixStore(PrefixStoreConfig{
		MaxEntries:   100,
		MinPrefixLen: 3,
		MaxPrefixLen: 10,
		EntryTTL:     1 * time.Hour,
	})

	// Create test pods
	pod1 := &types.PodMetrics{
		Pod: &backendmetrics.Pod{
			NamespacedName: k8stypes.NamespacedName{
				Name:      "pod1",
				Namespace: "default",
			},
		},
		Metrics: &backendmetrics.Metrics{},
	}
	pod2 := &types.PodMetrics{
		Pod: &backendmetrics.Pod{
			NamespacedName: k8stypes.NamespacedName{
				Name:      "pod2",
				Namespace: "default",
			},
		},
		Metrics: &backendmetrics.Metrics{},
	}

	tests := []struct {
		name           string
		weight         float64
		prompt         string
		modelName      string
		prefixToAdd    string
		podToAdd       k8stypes.NamespacedName
		prefixModel    string // Model name to use when adding the prefix
		expectedScores []float64
	}{
		{
			name:           "no prompt",
			weight:         1.0,
			prompt:         "",
			modelName:      "model1",
			prefixToAdd:    "hello",
			podToAdd:       pod1.Pod.NamespacedName,
			prefixModel:    "model1",
			expectedScores: []float64{0, 0}, // No prompt means zero scores
		},
		{
			name:           "exact prefix match",
			weight:         1.0,
			prompt:         "hello world",
			modelName:      "model1",
			prefixToAdd:    "hello",
			podToAdd:       pod1.Pod.NamespacedName,
			prefixModel:    "model1",
			expectedScores: []float64{1.0, 0}, // pod1 matches, pod2 doesn't
		},
		{
			name:           "no prefix match",
			weight:         1.0,
			prompt:         "goodbye",
			modelName:      "model1",
			prefixToAdd:    "hello",
			podToAdd:       pod1.Pod.NamespacedName,
			prefixModel:    "model1",
			expectedScores: []float64{0, 0}, // No matching prefix
		},
		{
			name:           "different model name",
			weight:         1.0,
			prompt:         "hello world",
			modelName:      "model2", // Try to find with model2
			prefixToAdd:    "hello",
			podToAdd:       pod1.Pod.NamespacedName,
			prefixModel:    "model1",        // But prefix was added with model1
			expectedScores: []float64{0, 0}, // Model name mismatch should result in no match
		},
		{
			name:           "custom weight",
			weight:         0.5,
			prompt:         "hello world",
			modelName:      "model1",
			prefixToAdd:    "hello",
			podToAdd:       pod1.Pod.NamespacedName,
			prefixModel:    "model1",
			expectedScores: []float64{0.5, 0}, // Weight affects score
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset prefix store for each test
			prefixStore = NewPrefixStore(PrefixStoreConfig{
				MaxEntries:   100,
				MinPrefixLen: 3,
				MaxPrefixLen: 10,
				EntryTTL:     1 * time.Hour,
			})

			// Add prefix if specified
			if tt.prefixToAdd != "" {
				err := prefixStore.AddPrefix(ctx, tt.prefixToAdd, tt.podToAdd, tt.prefixModel)
				if err != nil {
					t.Fatalf("Failed to add prefix: %v", err)
				}
			}

			// Create scorer with test weight
			scorer := NewPrefixAwareScorer(tt.weight, prefixStore)

			// Create test context
			sCtx := types.NewContext(ctx, &types.LLMRequest{
				Prompt:              tt.prompt,
				ResolvedTargetModel: tt.modelName,
			}, []*types.PodMetrics{})

			// Score pods
			pods := []*types.PodMetrics{pod1, pod2}
			scores, err := scorer.ScoreTargets(sCtx, pods)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// Verify scores
			if len(scores) != len(tt.expectedScores) {
				t.Fatalf("Expected %d scores, got %d", len(tt.expectedScores), len(scores))
			}

			for i, score := range scores {
				if score.Score != tt.expectedScores[i] {
					t.Errorf("Pod %d: expected score %v, got %v", i, tt.expectedScores[i], score.Score)
				}
			}
		})
	}
}
