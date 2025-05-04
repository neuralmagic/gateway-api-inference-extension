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

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/scheduling/plugins"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/scheduling/plugins/filter"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/scheduling/plugins/picker"
	logutil "sigs.k8s.io/gateway-api-inference-extension/pkg/epp/util/logging"
)

var prefillConfig = &SchedulerConfig{
	preSchedulePlugins:  []plugins.PreSchedule{},
	filters:             []plugins.Filter{filter.PrefillFilter},
	scorers:             map[plugins.Scorer]int{},
	picker:              picker.NewMaxScorePicker(),
	postSchedulePlugins: []plugins.PostSchedule{},
}
var decodeConfig = &SchedulerConfig{
	preSchedulePlugins:  []plugins.PreSchedule{},
	filters:             []plugins.Filter{filter.DecodeFilter},
	scorers:             map[plugins.Scorer]int{},
	picker:              picker.NewMaxScorePicker(),
	postSchedulePlugins: []plugins.PostSchedule{},
}

var IsPDEnabled = false
var PromptLengthThreshold int

func init() {
	ctx := context.Background()
	loggerDebug := log.FromContext(ctx).WithName("scheduler_config").V(logutil.DEBUG)

	loadPrefillConfiguration(ctx, loggerDebug)
	loadDecodeConfiguration(ctx, loggerDebug)

	// set IsPDEnabled by environment
	IsPDEnabled = getPDEnabledFromEnvironment(loggerDebug)
	PromptLengthThreshold = getPDPromptLenThresholdFromEnvironment(loggerDebug)
}

func loadPrefillConfiguration(ctx context.Context, logger logr.Logger) {
	// add scorers
	addScorerByEnvironment(ctx, prefillConfig, kvCacheAwareScorerName, kvCacheScorerEnablementEnvVar, kvCacheScorerWeightEnvVar, logger)
	addScorerByEnvironment(ctx, prefillConfig, loadAwareScorerName, loadAwareScorerEnablementEnvVar, loadAwareScorerWeightEnvVar, logger)

	// set filter
	// TODO - do we want to keep default filters?
	prefillConfig.filters = []plugins.Filter{filter.PrefillFilter}
}

func loadDecodeConfiguration(ctx context.Context, logger logr.Logger) {
	// add scorers
	addScorerByEnvironment(ctx, decodeConfig, kvCacheAwareScorerName, kvCacheScorerEnablementEnvVar, kvCacheScorerWeightEnvVar, logger)
	addScorerByEnvironment(ctx, decodeConfig, loadAwareScorerName, loadAwareScorerEnablementEnvVar, loadAwareScorerWeightEnvVar, logger)

	// set filter
	// TODO - do we want to keep default filters?
	decodeConfig.filters = []plugins.Filter{filter.DecodeFilter}
}
