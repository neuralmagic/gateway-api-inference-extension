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

// ActiveLorasScorer is a routing scorer that simply causes the
// request to be routed to one of the pods on which the specified
// model is one of currently active LoRA adapters.
// All pods that have this LoRA running now are returned as candidates
// to be the request target.
type ActiveLorasScorer struct {
	weight float64
}

func NewActiveLorasScorer(weight float64) Scorer {
	return ActiveLorasScorer{
		weight: weight,
	}
}

func (s ActiveLorasScorer) GetName() string {
	return "active loras scorer"
}

// ScoreTargets does the actual scoring of the target pods by the session affinity.
func (s ActiveLorasScorer) ScoreTargets(ctx *types.Context, pods []*types.PodMetrics) ([]PodScore, error) {
	logger := log.FromContext(ctx)

	logger.Info(">>> Check lora on pods", "lora", ctx.Req.Model)

	scoredPods := make([]PodScore, len(pods))

	for i, pod := range pods {
		logger.Info(">>> Check lora on pod", "lora", ctx.Req.Model, "pod", pod.NamespacedName.String())
		if _, ok := pod.Metrics.ActiveModels[ctx.Req.Model]; ok {
			// lora is running on this pod
			scoredPods[i].Score = s.weight
			logger.Info("Lora is running on a pod", "lora", ctx.Req.Model, "pod", pod.NamespacedName.String())
		}
		scoredPods[i].Pod = pod
	}

	return scoredPods, nil
}
