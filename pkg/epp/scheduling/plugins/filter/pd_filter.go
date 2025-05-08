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
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/backend/metrics"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/scheduling/types"
)

// PrefillFilter - filters out all pods that are not marked as decode/both pod role
var PrefillFilter = &baseFilter{
	name:   "prefill_filter",
	filter: prefillFilterFunc,
}

// prefillFilterFunc filters out all pods that are not marked as "prefill"
func prefillFilterFunc(ctx *types.SchedulingContext, pods []types.Pod) []types.Pod {
	filteredPods := make([]types.Pod, 0)

	for _, pod := range pods {
		if pod.GetPod().Role == metrics.Prefill {
			filteredPods = append(filteredPods, pod)
		}
	}

	return filteredPods
}

// DecodeFilter - fiters out all pods that are not marked as prefill pod role
var DecodeFilter = &baseFilter{
	name:   "decode_filter",
	filter: decodeFilterFunc,
}

// decodeFilterFunc filters out all pods that are not marked as "decode" or "both"
func decodeFilterFunc(ctx *types.SchedulingContext, pods []types.Pod) []types.Pod {
	filteredPods := make([]types.Pod, 0)

	for _, pod := range pods {
		if pod.GetPod().Role == metrics.Decode || pod.GetPod().Role == metrics.Both {
			filteredPods = append(filteredPods, pod)
		}
	}

	return filteredPods
}
