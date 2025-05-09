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

	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/scheduling/plugins/picker"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/scheduling/plugins/scorer"
	envutil "sigs.k8s.io/gateway-api-inference-extension/pkg/epp/util/env"
	logutil "sigs.k8s.io/gateway-api-inference-extension/pkg/epp/util/logging"
)

const (
	kvCacheScorerEnablementEnvVar      = "ENABLE_KVCACHE_AWARE_SCORER"
	loadAwareScorerEnablementEnvVar    = "ENABLE_LOAD_AWARE_SCORER"
	prefixScorerEnablementEnvVar       = "ENABLE_PREFIX_AWARE_SCORER"
	sessionAwareScorerEnablementEnvVar = "ENABLE_SESSION_AWARE_SCORER"
	pdFilterEnablementEnvVar           = "ENABLE_PD_FILTER"

	kvCacheScorerWeightEnvVar      = "KVCACHE_AWARE_SCORER_WEIGHT"
	loadAwareScorerWeightEnvVar    = "LOAD_AWARE_SCORER_WEIGHT"
	prefixScorerWeightEnvVar       = "PREFIX_AWARE_SCORER_WEIGHT"
	sessionAwareScorerWeightEnvVar = "SESSION_AWARE_SCORER_WEIGHT"
)

func init() {
	setDefaultConfig()
}

func setDefaultConfig() {
	// since the default config is a global variable, we add this function to minimize rebase conflicts.
	// this configuration is a temporary state, it should be better streamlined.
	setLoadAwareScorer()
	setSessionAwareScorer()
	setKVCacheAwareScorer()
	setPrefixScorer()

	defaultConfig.picker = picker.NewMaxScorePicker()
}

func setLoadAwareScorer() {
	ctx := context.Background()
	loggerDebug := log.FromContext(ctx).WithName("scheduler_config").V(logutil.DEBUG)

	if envutil.GetEnvString(loadAwareScorerEnablementEnvVar, "false", loggerDebug) != "true" {
		loggerDebug.Info("Skipping LoadAwareScorer creation as it is not enabled")
		return
	}

	loadBasedScorerWeight := envutil.GetEnvInt(loadAwareScorerWeightEnvVar, 1, loggerDebug)
	defaultConfig.scorers[&scorer.LoadAwareScorer{}] = loadBasedScorerWeight
	loggerDebug.Info("Initialized LoadAwareScorer", "weight", loadBasedScorerWeight)
}

func setSessionAwareScorer() {
	ctx := context.Background()
	loggerDebug := log.FromContext(ctx).WithName("scheduler_config").V(logutil.DEBUG)

	if envutil.GetEnvString(sessionAwareScorerEnablementEnvVar, "false", loggerDebug) != "true" {
		loggerDebug.Info("Skipping SessionAwareScorer creation as it is not enabled")
		return
	}

	sessionBasedScorerWeight := envutil.GetEnvInt(sessionAwareScorerWeightEnvVar, 1, loggerDebug)
	sessionAffinity := scorer.NewSessionAffinity()

	defaultConfig.scorers[sessionAffinity] = sessionBasedScorerWeight
	defaultConfig.postResponsePlugins = append(defaultConfig.postResponsePlugins, sessionAffinity)
	loggerDebug.Info("Initialized SessionAwareScorer", "weight", sessionBasedScorerWeight)
}

func setKVCacheAwareScorer() {
	ctx := context.Background()
	loggerDebug := log.FromContext(ctx).WithName("scheduler_config").V(logutil.DEBUG)

	if envutil.GetEnvString(kvCacheScorerEnablementEnvVar, "false", loggerDebug) != "true" {
		loggerDebug.Info("Skipping KVCacheAwareScorer creation as it is not enabled")
		return
	}

	kvCacheScorer, err := scorer.NewKVCacheAwareScorer(ctx)
	if err != nil {
		loggerDebug.Error(err, "Failed to create KVCacheAwareScorer")
		return
	}

	kvCacheScorerWeight := envutil.GetEnvInt(kvCacheScorerWeightEnvVar, 1, loggerDebug)
	defaultConfig.scorers[kvCacheScorer] = kvCacheScorerWeight
	loggerDebug.Info("Initialized KVCacheAwareScorer", "weight", kvCacheScorerWeight)
}

func setPrefixScorer() {
	ctx := context.Background()
	loggerDebug := log.FromContext(ctx).WithName("scheduler_config").V(logutil.DEBUG)

	if envutil.GetEnvString(prefixScorerEnablementEnvVar, "false", loggerDebug) != "true" {
		loggerDebug.Info("Skipping PrefixScorer creation as it is not enabled")
		return
	}

	prefixScorerWeight := envutil.GetEnvInt(prefixScorerWeightEnvVar, 1, loggerDebug)
	prefixScorer := scorer.NewPrefixAwareScorer(nil)
	defaultConfig.scorers[prefixScorer] = prefixScorerWeight // TODO: make configurable
	defaultConfig.postSchedulePlugins = append(defaultConfig.postSchedulePlugins, prefixScorer)

	loggerDebug.Info("Initialized PrefixAwareScorer", "weight", prefixScorerWeight)
}
