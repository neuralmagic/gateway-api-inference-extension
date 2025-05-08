/*
Copyright 2025 The Neural Magic Authors.

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
	"context"
	"fmt"
	"os"

	kvcache "github.com/neuralmagic/llm-d-kv-cache-manager/pkg/kv-cache"

	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/scheduling/plugins"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/scheduling/types"

	logutil "sigs.k8s.io/gateway-api-inference-extension/pkg/epp/util/logging"
)

const (
	kvCacheAwareScorerName = "kvcache-aware-scorer"
	kvCacheRedisEnvVar     = "KVCACHE_INDEXER_REDIS_ADDR"
	huggingFaceTokenEnvVar = "HF_TOKEN"
)

// KVCacheAwareScorer uses the KVCacheIndexer to score pods based on KVCache
// awareness.
type KVCacheAwareScorer struct {
	kvCacheIndexer *kvcache.Indexer
}

// NewKVCacheAwareScorer creates a new KVCacheAwareScorer instance.
// It initializes the KVCacheIndexer from environment variables.
//
// If the environment variables are not set, or if the indexer
// fails to initialize, an error is returned.
func NewKVCacheAwareScorer(ctx context.Context) (plugins.Scorer, error) {
	config := kvcache.NewDefaultConfig()

	redisAddr := os.Getenv(kvCacheRedisEnvVar)
	if redisAddr != "" {
		config.KVBlockIndexerConfig.RedisKVBlockIndexerConfig.RedisAddr = redisAddr
	} else {
		return nil, fmt.Errorf("environment variable %s is not set", kvCacheRedisEnvVar)
	}

	hfToken := os.Getenv(huggingFaceTokenEnvVar)
	if hfToken != "" {
		config.TokenizersPoolConfig.HFTokenizerConfig.HuggingFaceToken = hfToken
	} else {
		return nil, fmt.Errorf("environment variable %s is not set", huggingFaceTokenEnvVar)
	}

	kvCacheIndexer, err := kvcache.NewKVCacheIndexer(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create KVCacheIndexer: %w", err)
	}

	go kvCacheIndexer.Run(ctx)

	return &KVCacheAwareScorer{
		kvCacheIndexer: kvCacheIndexer,
	}, nil
}

// Name returns the name of the scorer.
func (s *KVCacheAwareScorer) Name() string {
	return kvCacheAwareScorerName
}

// Score scores the provided pod based on the KVCache index state.
// The returned scores are normalized to a range of 0-1.
func (s *KVCacheAwareScorer) Score(ctx *types.SchedulingContext, pods []types.Pod) map[types.Pod]float64 {
	loggerDebug := log.FromContext(ctx).WithName(kvCacheAwareScorerName).V(logutil.DEBUG)
	if ctx.Req == nil {
		loggerDebug.Info("Request is nil, skipping scoring")
		return nil
	}

	scores, err := s.kvCacheIndexer.GetPodScores(ctx.Context, ctx.Req.Prompt, ctx.Req.Model, nil)
	if err != nil {
		loggerDebug.Error(err, "Failed to get pod scores")
		return nil
	}
	loggerDebug.Info("Got pod scores", "scores", scores)

	return indexerScoresToNormalizedScoredPods(pods, scores)
}

func getMinMax(scores map[string]int) (int, int) {
	minScore := int(^uint(0) >> 1) // max int
	maxScore := -1

	for _, score := range scores {
		if score < minScore {
			minScore = score
		}
		if score > maxScore {
			maxScore = score
		}
	}

	return minScore, maxScore
}

func indexerScoresToNormalizedScoredPods(pods []types.Pod, scores map[string]int) map[types.Pod]float64 {
	scoredPods := make(map[types.Pod]float64)
	minScore, maxScore := getMinMax(scores)

	for _, pod := range pods {
		metricsPod := pod.GetPod()
		if metricsPod == nil {
			continue
		}

		if score, ok := scores[metricsPod.Address]; ok {
			if minScore == maxScore {
				scoredPods[pod] = 1.0
				continue
			}

			scoredPods[pod] = float64(score-minScore) / float64(maxScore-minScore)
		} else {
			scoredPods[pod] = 0.0
		}
	}

	return scoredPods
}
