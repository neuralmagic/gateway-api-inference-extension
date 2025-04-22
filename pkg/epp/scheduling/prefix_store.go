package scheduling

import (
	"context"
	"sync"
	"time"

	"github.com/armon/go-radix"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// PrefixEntry represents a single entry in the prefix store
type PrefixEntry struct {
	PodRef    types.NamespacedName
	LastUsed  time.Time
	ModelName string
}

// PrefixStoreConfig holds configuration for the prefix store
type PrefixStoreConfig struct {
	MaxEntries   int           // Maximum total entries in the store
	MinPrefixLen int           // Minimum prefix length to store
	MaxPrefixLen int           // Maximum prefix length to store
	EntryTTL     time.Duration // Time-to-live for entries
}

// PrefixStore manages prompt prefixes and their pod assignments
type PrefixStore struct {
	tree   *radix.Tree
	mu     sync.RWMutex
	config PrefixStoreConfig
}

// NewPrefixStore creates a new PrefixStore with the given configuration
func NewPrefixStore(config PrefixStoreConfig) *PrefixStore {
	return &PrefixStore{
		tree:   radix.New(),
		config: config,
	}
}

// AddPrefix adds or updates a prefix entry in the store
func (ps *PrefixStore) AddPrefix(ctx context.Context, prefix string, pod types.NamespacedName, modelName string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	logger := log.FromContext(ctx)

	// Validate prefix length
	if len(prefix) < ps.config.MinPrefixLen {
		return ErrPrefixTooShort
	}
	if len(prefix) > ps.config.MaxPrefixLen {
		prefix = prefix[:ps.config.MaxPrefixLen]
	}

	// Check if we're updating an existing entry
	if val, exists := ps.tree.Get(prefix); exists {
		entry := val.(*PrefixEntry)
		if entry.PodRef == pod && entry.ModelName == modelName {
			entry.LastUsed = time.Now()
			ps.tree.Insert(prefix, entry)
			return nil
		}
	}

	// Check total entries limit
	if ps.tree.Len() >= ps.config.MaxEntries {
		ps.evictOldest()
	}

	// Add new entry
	entry := &PrefixEntry{
		PodRef:    pod,
		LastUsed:  time.Now(),
		ModelName: modelName,
	}
	ps.tree.Insert(prefix, entry)

	logger.Info("Added prefix entry", "prefix", prefix, "pod", pod.String(), "model", modelName)
	return nil
}

// FindPodForPrefix finds the best matching pod for a given prefix and model
func (ps *PrefixStore) FindPodForPrefix(ctx context.Context, prefix string, modelName string) (types.NamespacedName, bool) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	logger := log.FromContext(ctx)

	if len(prefix) < ps.config.MinPrefixLen {
		return types.NamespacedName{}, false
	}

	if len(prefix) > ps.config.MaxPrefixLen {
		prefix = prefix[:ps.config.MaxPrefixLen]
	}

	// Use LongestPrefix to find the best match
	matchedPrefix, val, found := ps.tree.LongestPrefix(prefix)
	if !found {
		return types.NamespacedName{}, false
	}

	entry := val.(*PrefixEntry)

	// Check if entry has expired or model doesn't match
	if time.Since(entry.LastUsed) > ps.config.EntryTTL || entry.ModelName != modelName {
		// Don't remove here to avoid write lock
		return types.NamespacedName{}, false
	}

	// Update LastUsed time for the matched entry
	entry.LastUsed = time.Now()
	ps.tree.Insert(matchedPrefix, entry)

	logger.Info("Found pod for prefix", "prefix", prefix, "matchedPrefix", matchedPrefix, "pod", entry.PodRef.String(), "model", modelName)
	return entry.PodRef, true
}

// evictOldest removes the oldest entry from the store
func (ps *PrefixStore) evictOldest() {
	var oldestKey string
	var oldestTime time.Time
	first := true

	// Use Walk to find the oldest entry
	ps.tree.Walk(func(key string, value interface{}) bool {
		entry := value.(*PrefixEntry)
		if first || entry.LastUsed.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.LastUsed
			first = false
		}
		return false // continue walking
	})

	if oldestKey != "" {
		ps.tree.Delete(oldestKey)
	}
}

// cleanupExpired removes expired entries
func (ps *PrefixStore) cleanupExpired(ctx context.Context) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	logger := log.FromContext(ctx)
	now := time.Now()
	var keysToDelete []string

	// Use Walk to find expired entries
	ps.tree.Walk(func(key string, value interface{}) bool {
		entry := value.(*PrefixEntry)
		if now.Sub(entry.LastUsed) > ps.config.EntryTTL {
			keysToDelete = append(keysToDelete, key)
		}
		return false
	})

	// Delete expired entries
	for _, key := range keysToDelete {
		ps.tree.Delete(key)
	}

	if len(keysToDelete) > 0 {
		logger.Info("Cleaned up expired entries", "count", len(keysToDelete))
	}
}

// RunMaintenance performs periodic cleanup of expired entries
func (ps *PrefixStore) RunMaintenance(ctx context.Context) {
	logger := log.FromContext(ctx)
	ticker := time.NewTicker(ps.config.EntryTTL / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Info("Maintenance routine stopping")
			return
		case <-ticker.C:
			ps.cleanupExpired(ctx)
			logger.Info("Completed maintenance cycle")
		}
	}
}
