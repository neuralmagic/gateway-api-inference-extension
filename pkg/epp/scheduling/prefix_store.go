package scheduling

import (
	"sync"
	"time"

	"github.com/armon/go-radix"
	"k8s.io/apimachinery/pkg/types"
)

type PrefixEntry struct {
	PodRef    types.NamespacedName
	LastUsed  time.Time
	ModelName string
}

type PrefixStoreConfig struct {
	MaxEntries   int
	MinPrefixLen int
	MaxPrefixLen int
	EntryTTL     time.Duration
}

type PrefixStore struct {
	tree   *radix.Tree
	mu     sync.RWMutex
	config PrefixStoreConfig
}

func NewPrefixStore(config PrefixStoreConfig) *PrefixStore {
	return &PrefixStore{
		tree:   radix.New(),
		config: config,
	}
}
