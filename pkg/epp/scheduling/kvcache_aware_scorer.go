package scheduling

import (
	"context"
	"errors"
	"fmt"
	"os"

	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/gateway-api-inference-extension/pkg/epp/scheduling/types"

	"github.com/neuralmagic/kvcache-manager/pkg/kvcache"
)

const kvCacheAwareScorerName = "KVCacheAwareScorer"

// KVCacheAwareScorer is a concrete implementation of the Scorer interface.
// It uses the KVCacheIndexer to score pods based on KVCache awareness.
type KVCacheAwareScorer struct {
	kvCacheIndexer *kvcache.Indexer
}

func kvCacheIndexerConfigFromEnv() *kvcache.Config {
	config := kvcache.NewDefaultConfig()

	redisAddr := os.Getenv("KVCACHE_INDEXER_REDIS_ADDR")
	if redisAddr != "" {
		config.KVBlockIndexerConfig.RedisKVBlockIndexerConfig.RedisAddr = redisAddr
	}

	hfToken := os.Getenv("HF_TOKEN")
	if hfToken != "" {
		config.TokenizersPoolConfig.HFTokenizerConfig.HuggingFaceToken = hfToken
	}

	return config
}

// NewKVCacheAwareScorer creates a new KVCacheAwareScorer instance.
// It initializes the KVCacheIndexer with the provided configuration,
// and runs it with the given context.
//
// If the configuration is nil, it uses the default configuration.
func NewKVCacheAwareScorer(ctx context.Context, config *kvcache.Config) (Scorer, error) {
	if config == nil {
		config = kvcache.NewDefaultConfig()
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

// ScoreTargets scores the provided pods based on their KVCache awareness.
func (s *KVCacheAwareScorer) ScoreTargets(ctx *types.Context, pods []*types.PodMetrics) ([]PodScore, error) {
	if ctx.Req == nil {
		return nil, errors.New("request (ctx.Req) is nil, cannot score pods")
	}
	logger := log.FromContext(ctx).WithName(kvCacheAwareScorerName)

	podIdentifiers, podIdentifierToPod := podMetricsToKeysAndMap(pods)
	if len(podIdentifiers) == 0 {
		return nil, nil
	}

	scores, err := s.kvCacheIndexer.GetPodScores(ctx.Context, ctx.Req.Prompt, ctx.Req.Model, podIdentifiers)
	if err != nil {
		return nil, fmt.Errorf("failed to get pod scores: %w", err)
	}

	scoredPods := make([]PodScore, 0, len(pods))
	for _, pod := range scores {
		podMetrics, ok := podIdentifierToPod[pod.Name]
		if !ok {
			continue
		}

		scoredPods = append(scoredPods, PodScore{
			Score: pod.Score,
			Pod:   podMetrics,
		})
	}

	logger.Info("Scored pods", "podScores", scoredPods)
	return scoredPods, nil
}

func podMetricsToKeysAndMap(pods []*types.PodMetrics) ([]string, map[string]*types.PodMetrics) {
	podIdentifiers := make([]string, 0, len(pods))
	podIdentifierToPod := make(map[string]*types.PodMetrics, len(pods))

	for _, pod := range pods {
		if pod == nil {
			continue
		}

		podIdentifier := pod.GetPod().Address
		podIdentifiers = append(podIdentifiers, podIdentifier)
		podIdentifierToPod[podIdentifier] = pod
	}

	return podIdentifiers, podIdentifierToPod
}
