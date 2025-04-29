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
	"math/rand/v2"

	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/backend/metrics"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/scheduling/types"
)

var PDFilter = &baseFilter{
	name:   "p/d filter",
	filter: prefillDecodeFilterFunc,
}

// prefillDecodeFilterFunc implements a pod selection strategy that filters out pods,
// which role is not 'prefill'
//
// Initial implementation:
// 1 - to filter out all pods that are not 'prefill'
// 2 - from the list of prefill pods select only one, which was sleected randomly
//
// Returns:
//   - Filtered slice of pod metrics, could contain one or zerro elements
func prefillDecodeFilterFunc(ctx *types.SchedulingContext, pods []types.Pod) []types.Pod {
	pPods := make([]types.Pod, 0)

	for _, pod := range pods {
		if pod.GetPod().Role == metrics.Prefill {
			pPods = append(pPods, pod)
		}
	}

	if len(pPods) > 1 {
		// leave only one pod
		randomIndex := rand.IntN(len(pPods))
		return []types.Pod{pPods[randomIndex]}
	}

	return []types.Pod{}
}
