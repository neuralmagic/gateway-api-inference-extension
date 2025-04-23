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
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/scheduling/types"
)

// PrefixAwareScorer is a routing scorer that scores pods based on the longest prefix match
// between the request's prompt and stored prefixes. The score is normalized between 0 and 1,
// where 1 represents the longest matching prefix.
type PrefixAwareScorer struct {
	weight      float64
	prefixStore *PrefixStore
}

// NewPrefixAwareScorer creates a new PrefixAwareScorer with the given weight and prefix store
func NewPrefixAwareScorer(weight float64, prefixStore *PrefixStore) Scorer {
	return &PrefixAwareScorer{
		weight:      weight,
		prefixStore: prefixStore,
	}
}

// ScoreTargets scores the target pods based on the longest prefix match
func (s *PrefixAwareScorer) ScoreTargets(ctx *types.Context, pods []*types.PodMetrics) ([]PodScore, error) {
	logger := log.FromContext(ctx)
	scoredPods := make([]PodScore, len(pods))

	// Get the prompt from the request
	prompt := ctx.Req.Prompt
	if prompt == "" {
		// If no prompt, return zero scores for all pods
		for i, pod := range pods {
			scoredPods[i] = PodScore{
				Score: 0,
				Pod:   pod,
			}
		}
		return scoredPods, nil
	}

	// Find the best matching pod for the prompt
	matchedPod, found := s.prefixStore.FindPodForPrefix(ctx, prompt, ctx.Req.ResolvedTargetModel)
	if !found {
		// If no matching prefix found, return zero scores for all pods
		for i, pod := range pods {
			scoredPods[i] = PodScore{
				Score: 0,
				Pod:   pod,
			}
		}
		return scoredPods, nil
	}

	// Assign scores based on pod match
	for i, pod := range pods {
		if pod.NamespacedName == matchedPod {
			logger.Info("Pod found for prefix", "prompt", prompt, "pod", pod.NamespacedName.String())
			scoredPods[i] = PodScore{
				Score: s.weight, // Use the configured weight for the matching pod
				Pod:   pod,
			}
		} else {
			scoredPods[i] = PodScore{
				Score: 0, // Zero score for non-matching pods
				Pod:   pod,
			}
		}
	}

	return scoredPods, nil
}
