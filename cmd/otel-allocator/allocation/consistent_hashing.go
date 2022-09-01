package allocation

import (
	"github.com/buraksezer/consistent"
	"github.com/cespare/xxhash/v2"
)

var _ AllocatorStrategy = ConsistentHashingStrategy{}

type hasher struct{}

func (h hasher) Sum64(data []byte) uint64 {
	return xxhash.Sum64(data)
}

type ConsistentHashingStrategy struct {
	config consistent.Config
}

func NewConsistentHashingStrategy() *ConsistentHashingStrategy {
	return &ConsistentHashingStrategy{
		config: consistent.Config{
			PartitionCount:    71,
			ReplicationFactor: 20,
			Load:              1.25,
			Hasher:            hasher{},
		}
	}
}

func (c ConsistentHashingStrategy) Allocate(currentState, newState State) State {
	nextState := currentState
	// Check for target changes
	targetsDiff := diff(currentState.targetItems, newState.targetItems)
	// If there are any additions or removals
	if len(targetsDiff.additions) != 0 || len(targetsDiff.removals) != 0 {
		nextState = l.handleTargets(targetsDiff, currentState)
	}
	// Check for collector changes
	collectorsDiff := diff(currentState.collectors, newState.collectors)
	// If there are any additions or removals
	if len(collectorsDiff.additions) != 0 || len(collectorsDiff.removals) != 0 {
		nextState = l.handleCollectors(collectorsDiff, nextState)
	}
	return nextState
}
