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
	"fmt"
	"math/rand/v2"

	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/backend/metrics"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/scheduling/types"
	logutil "sigs.k8s.io/gateway-api-inference-extension/pkg/epp/util/logging"
)

const (
	prefillPodHeader = "x-prefiller-url"
)

var PDFilter = &baseFilter{
	name:   "p/d filter",
	filter: prefillDecodeFilterFunc,
}

// prefillDecodeFilterFunc implements a pod selection strategy that filters out pods,
// which role is 'prefill', in addition a header with selected prefill pod is added
//
// Initial implementation:
// 1 - select one random pod marked as 'prefill' and add it name to header
// 2 - return a random pod that marked as "decode" or "both"
//
// Returns:
//   - Filtered slice of pod metrics, could contain one or zerro elements
func prefillDecodeFilterFunc(ctx *types.SchedulingContext, pods []types.Pod) []types.Pod {
	logger := log.FromContext(ctx).WithName("p/d filter").V(logutil.DEBUG)

	pPods := make([]types.Pod, 0)
	dPods := make([]types.Pod, 0)

	for _, pod := range pods {
		if pod.GetPod().Role == metrics.Prefill {
			pPods = append(pPods, pod)
		} else if pod.GetPod().Role == metrics.Decode || pod.GetPod().Role == metrics.Both {
			dPods = append(dPods, pod)
		}
	}

	if len(pPods) > 0 {
		// select a random prefill pod
		randomIndex := rand.IntN(len(pPods))
		url := fmt.Sprintf("http://%s:%d", pPods[randomIndex].GetPod().Address, ctx.TargetPort)
		logger.Info("prefill pod selected", "url", url)

		ctx.MutatedHeaders[prefillPodHeader] = url
	}

	if len(dPods) > 1 {
		// leave only one pod
		randomIndex := rand.IntN(len(dPods))
		return []types.Pod{dPods[randomIndex]}
	}

	return dPods
}
