// Package lru implements an LRU cache optimized for concurrent reads
package nonblocking

import (
	"errors"
	"sync"
)

// EvictCallback is used to get a callback when a cache entry is evicted
type EvictCallback[K comparable, V any] func(key K, value V)

// LRU implements a thread-safe fixed size LRU cache optimized for reads
type LRU[K comparable, V any] struct {
	size      int
	lock      sync.RWMutex // Main lock for structural changes
	items     map[K]*entry[K, V]
	evictList *lruList[K, V]
	onEvict   EvictCallback[K, V]

	// Buffered updates for list operations
	updateBatch  []*entry[K, V]
	batchLock    sync.Mutex
	batchSize    int
	updateSignal chan struct{}
	done         chan struct{}
}

func NewLRU[K comparable, V any](size int, onEvict EvictCallback[K, V]) (*LRU[K, V], error) {
	if size <= 0 {
		return nil, errors.New("must provide a positive size")
	}

	batchSize := size / 20 // 5% of cache size for batch operations
	if batchSize < 10 {
		batchSize = 10
	}

	c := &LRU[K, V]{
		size:         size,
		evictList:    newList[K, V](),
		items:        make(map[K]*entry[K, V], size),
		onEvict:      onEvict,
		updateBatch:  make([]*entry[K, V], 0, batchSize),
		batchSize:    batchSize,
		updateSignal: make(chan struct{}, 1),
		done:         make(chan struct{}),
	}

	go c.processBatchUpdates()
	return c, nil
}

// Get looks up a key's value from the cache
func (c *LRU[K, V]) Get(key K) (value V, ok bool) {
	// Fast path: Read-only lookup
	c.lock.RLock()
	if ent, exists := c.items[key]; exists {
		value = ent.value // Direct value access
		c.lock.RUnlock()

		// Queue update for batch processing
		c.queueUpdate(ent)
		return value, true
	}
	c.lock.RUnlock()
	return value, false
}

// queueUpdate adds an entry to the batch update queue
func (c *LRU[K, V]) queueUpdate(ent *entry[K, V]) {
	c.batchLock.Lock()
	c.updateBatch = append(c.updateBatch, ent)

	// Signal if batch is full
	if len(c.updateBatch) >= c.batchSize {
		select {
		case c.updateSignal <- struct{}{}:
		default:
		}
	}
	c.batchLock.Unlock()
}

// processBatchUpdates handles batched LRU list updates
func (c *LRU[K, V]) processBatchUpdates() {
	for {
		select {
		case <-c.done:
			return
		case <-c.updateSignal:
			c.processBatch()
		}
	}
}

func (c *LRU[K, V]) processBatch() {
	c.batchLock.Lock()
	batch := c.updateBatch
	c.updateBatch = make([]*entry[K, V], 0, c.batchSize)
	c.batchLock.Unlock()

	if len(batch) == 0 {
		return
	}

	c.lock.Lock()
	for _, ent := range batch {
		c.evictList.moveToFront(ent)
	}
	c.lock.Unlock()
}

// Add adds a value to the cache
func (c *LRU[K, V]) Add(key K, value V) (evicted bool) {
	c.lock.Lock()
	defer c.lock.Unlock()

	// Check for existing item
	if ent, ok := c.items[key]; ok {
		c.evictList.moveToFront(ent)
		ent.value = value
		return false
	}

	// Add new item
	ent := c.evictList.pushFront(key, value)
	c.items[key] = ent

	// Remove oldest if needed
	if c.evictList.length() > c.size {
		oldest := c.evictList.back()
		if oldest != nil {
			c.removeElement(oldest)
			return true
		}
	}

	return false
}

// removeElement removes an element from the cache
func (c *LRU[K, V]) removeElement(e *entry[K, V]) {
	c.evictList.remove(e)
	delete(c.items, e.key)
	if c.onEvict != nil {
		c.onEvict(e.key, e.value)
	}
}

// Len returns the number of items in the cache
func (c *LRU[K, V]) Len() int {
	c.lock.RLock()
	defer c.lock.RUnlock()
	return c.evictList.length()
}

// Resize changes the cache size
func (c *LRU[K, V]) Resize(size int) (evicted int) {
	c.lock.Lock()
	defer c.lock.Unlock()

	c.size = size
	diff := c.evictList.length() - size
	if diff < 0 {
		diff = 0
	}

	for i := 0; i < diff; i++ {
		if ent := c.evictList.back(); ent != nil {
			c.removeElement(ent)
		}
	}
	return diff
}

// Close stops the background goroutine
func (c *LRU[K, V]) Close() {
	close(c.done)
}
