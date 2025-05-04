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
	errutil "sigs.k8s.io/gateway-api-inference-extension/pkg/epp/util/error"
)

const (
	prefillPodHeader = "x-prefiller-url"
)

func NewPDScheduler(datastore Datastore) *PDScheduler {
	return NewPDSchedulerWithConfig(datastore, prefillConfig, decodeConfig)
}

func NewPDSchedulerWithConfig(datastore Datastore, prefillConfig *SchedulerConfig, decodeConfig *SchedulerConfig) *PDScheduler {
	return &PDScheduler{
		datastore:        datastore,
		prefillScheduler: NewSchedulerWithConfig(datastore, prefillConfig),
		decodeScheduler:  NewSchedulerWithConfig(datastore, decodeConfig),
	}
}

type PDScheduler struct {
	datastore        Datastore
	prefillScheduler *Scheduler
	decodeScheduler  *Scheduler
}

// Schedule finds the target pod based on metrics and the requested lora adapter.
func (s *PDScheduler) Schedule(ctx context.Context, req *types.LLMRequest) (*types.Result, error) {
	logger := log.FromContext(ctx).WithValues("pd-schedule", req)

	if len(req.Prompt) < PromptLengthThreshold {
		// prompt is short enough - use decode scheduling logic
		return s.decodeScheduler.Schedule(ctx, req)
	}

	pool, err := s.datastore.PoolGet()
	if err != nil {
		return nil, errutil.Error{Code: errutil.Internal, Msg: "failed to find a target pod"} // pool not defined, no pods
	}

	// Snapshot pod metrics from the datastore to:
	// 1. Reduce concurrent access to the datastore.
	// 2. Ensure consistent data during the scheduling operation of a request.
	sCtx := types.NewSchedulingContext(ctx, req, types.ToSchedulerPodMetrics(s.datastore.PodGetAll()), pool.Spec.TargetPortNumber)

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
