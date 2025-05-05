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

// Package scheduling implements request scheduling algorithms.
package scheduling

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/scheduling/types"
)

const (
	prefillPodHeader = "x-prefiller-url"
)

func NewPDScheduler(datastore Datastore) *PDScheduler {
	return NewPDSchedulerWithConfig(datastore, prefillConfig, decodeConfig, defaultConfig)
}

func NewPDSchedulerWithConfig(datastore Datastore, pConfig *SchedulerConfig, dConfig *SchedulerConfig, defConfig *SchedulerConfig) *PDScheduler {
	return &PDScheduler{
		datastore:        datastore,
		prefillScheduler: NewSchedulerWithConfig(datastore, pConfig),
		decodeScheduler:  NewSchedulerWithConfig(datastore, dConfig),
		defaultScheduler: NewSchedulerWithConfig(datastore, defConfig),
	}
}

type PDScheduler struct {
	datastore        Datastore
	prefillScheduler *Scheduler
	decodeScheduler  *Scheduler
	defaultScheduler *Scheduler
}

// Schedule finds the target pod based on metrics and the requested lora adapter.
// PD scheduler uses three base schedulers to process requests, the overall configuration is currently loaded from environment variables.
// If the request prompt is short enough (defined by the threshold in the configuration) - use the default behavior
// If the request prompt is long enough to use prefill-decode process:
// 1 - find the pod for prefill, save its url in a special header. For this, use the Scheduler configured for this goal, which uses the prefill filter
// and scorers according to the configuration.
// 2 - find the pod for decode, use the Scheduler configured for this goal, which uses the decode filer and scorers defined in the configuration
func (s *PDScheduler) Schedule(ctx context.Context, req *types.LLMRequest) (*types.Result, error) {
	logger := log.FromContext(ctx).WithValues("pd-schedule", req)

	if len(req.Prompt) < promptLengthThreshold {
		// the prompt is short enough - use the default scheduling logic
		return s.defaultScheduler.Schedule(ctx, req)
	}

	sCtx, err := createSchedulerContext(ctx, req, s.datastore)
	if err != nil {
		return nil, err
	}

	// prompt requires processing on two pods - prefill and decode
	// start with calculating of the prefill pod
	res, err := s.prefillScheduler.scheduleWithContext(ctx, sCtx, req, logger)
	if err != nil {
		return nil, err
	}

	if res.TargetPod != nil {
		url := fmt.Sprintf("http://%s:%d", res.TargetPod.GetPod().Address, sCtx.TargetPort)
		sCtx.MutatedHeaders[prefillPodHeader] = url
	}

	// get decode pod
	return s.decodeScheduler.scheduleWithContext(ctx, sCtx, req, logger)
}

func (s *PDScheduler) RunPostResponsePlugins(ctx context.Context, req *types.LLMRequest, targetPodName string) (*types.Result, error) {
	return s.decodeScheduler.RunPostResponsePlugins(ctx, req, targetPodName)
}
