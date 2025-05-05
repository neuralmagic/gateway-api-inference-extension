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
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/scheduling/plugins"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/scheduling/plugins/scorer"
	envutil "sigs.k8s.io/gateway-api-inference-extension/pkg/epp/util/env"
)

const (
	prefillKvCacheScorerEnablementEnvVar   = "PREFILL_ENABLE_KVCACHE_AWARE_SCORER"
	prefillLoadAwareScorerEnablementEnvVar = "PREFILL_ENABLE_LOAD_AWARE_SCORER"
	decodeKvCacheScorerEnablementEnvVar    = "DECODE_ENABLE_KVCACHE_AWARE_SCORER"
	decodeLoadAwareScorerEnablementEnvVar  = "DECODE_ENABLE_LOAD_AWARE_SCORER"

	prefillKvCacheScorerWeightEnvVar   = "PREFILL_KVCACHE_AWARE_SCORER_WEIGHT"
	prefillLoadAwareScorerWeightEnvVar = "PREFILL_LOAD_AWARE_SCORER_WEIGHT"
	decodeKvCacheScorerWeightEnvVar    = "DECODE_KVCACHE_AWARE_SCORER_WEIGHT"
	decodeLoadAwareScorerWeightEnvVar  = "DECODE_LOAD_AWARE_SCORER_WEIGHT"

	pdEnabledEnvKey = "PD_ENABLED"

	pdPromptLenThresholdEnvKey  = "PD_PROMPT_LEN_THRESHOLD"
	pdPromptLenThresholdDefault = 10
)

const (
	loadAwareScorerName    = "LoadAwareScorer"
	kvCacheAwareScorerName = "KVCacheAwareScorer"
)

func addScorerByEnvironment(ctx context.Context, config *SchedulerConfig, scorerName string, scorerEnabledEnvKey string, weightEnvKey string, logger logr.Logger) {
	if envutil.GetEnvString(scorerEnabledEnvKey, "false", logger) != "true" {
		logger.Info(fmt.Sprintf("Skipping %s creation as it is not enabled", scorerName))
		return
	}

	weight := envutil.GetEnvInt(weightEnvKey, 1, logger)
	scorer, err := createScorerByName(ctx, scorerName)
	if err != nil {
		logger.Error(err, "Failed to create scorrer")
		return
	}

	defaultConfig.scorers[scorer] = weight
	logger.Info("Initialized scorer", "scorer", scorerName, "weight", weight)
}

func createScorerByName(ctx context.Context, name string) (plugins.Scorer, error) {
	switch name {
	case loadAwareScorerName:
		return &scorer.LoadAwareScorer{}, nil
	case kvCacheAwareScorerName:
		return scorer.NewKVCacheAwareScorer(ctx)
	}
	return nil, fmt.Errorf("invalid scorer type %s", name)
}

func getPDEnabledFromEnvironment(logger logr.Logger) bool {
	return envutil.GetEnvString(pdEnabledEnvKey, "false", logger) == "true"
}

func getPDPromptLenThresholdFromEnvironment(logger logr.Logger) int {
	return envutil.GetEnvInt(pdPromptLenThresholdEnvKey, pdPromptLenThresholdDefault, logger)
}
