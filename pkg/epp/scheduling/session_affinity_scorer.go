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
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/scheduling/types"
)

// sessionAffinity is a routing scorer that routes subsequent
// requests in a session to the same pod as the first request in the
// session was sent to, by giving that pod the specified weight and assigning
// zero score to the rest of the targets
type SessionAffinityScorer struct {
	weight    float64
	datastore Datastore
}

func NewSessionAffinityScorer(weight float64, datastore Datastore) types.Scorer {
	return SessionAffinityScorer{
		weight:    weight,
		datastore: datastore,
	}
}

func (s SessionAffinityScorer) Name() string {
	return "session affinity scorer"
}

func (s SessionAffinityScorer) Score(ctx *types.Context, pod types.Pod) (float64, error) {
	score := 0.0

	if ctx.Req.SessionID != "" {
		podForSession := s.datastore.GetPodForSession(ctx.Req.SessionID)
		if podForSession != nil {
			if podForSession.NamespacedName.String() == pod.GetPod().NamespacedName.String() {
				score = 1.0
			}
		}
	}

	return score, nil
}
