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

package scorer

import (
	"encoding/base64"
	"time"

	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/scheduling/types"
)

const (
	sessionKeepAliveTime           = 60 * time.Minute  // How long should an idle session be kept alive
	sessionKeepAliveCheckFrequency = 15 * time.Minute  // How often to check for overly idle sessions
	sessionTokenHeader             = "x-session-token" // name of the session header in request
)

// sessionAffinity is a routing scorer that routes subsequent
// requests in a session to the same pod as the first request in the
// session was sent to, by giving that pod the specified weight and assigning
// zero score to the rest of the targets
type SessionAffinity struct {
}

func NewSessionAffinity() *SessionAffinity {
	return &SessionAffinity{}
}

func (s *SessionAffinity) Name() string {
	return "session affinity scorer"
}

func (s *SessionAffinity) Score(ctx *types.SchedulingContext, pods []types.Pod) map[types.Pod]float64 {
	scoredPods := make(map[types.Pod]float64)

	reqHeaders := ctx.Req.Headers

	var sessionToken = ""
	v, ok := reqHeaders[sessionTokenHeader]
	if ok {
		sessionToken = v
	}

	podName := ""
	if sessionToken != "" {
		decodedBytes, err := base64.StdEncoding.DecodeString(sessionToken)
		if err != nil {
			ctx.Logger.Error(err, "Error decoding")
		} else {
			podName = string(decodedBytes)
		}
	}
	for _, pod := range pods {
		if podName == "" {
			scoredPods[pod] = 0.0
		} else {
			if pod.GetPod().NamespacedName.String() == podName {
				scoredPods[pod] = 1.0
			}
		}
	}

	return scoredPods
}

func (s *SessionAffinity) PostResponse(ctx *types.SchedulingContext, pod types.Pod) {
	ctx.MutatedHeaders[sessionTokenHeader] = base64.StdEncoding.EncodeToString([]byte(pod.GetPod().NamespacedName.String()))
}
