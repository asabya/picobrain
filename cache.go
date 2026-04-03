package picobrain

import (
	"container/list"
	"sync"
	"time"
)

type thoughtCacheEntry struct {
	thought    Thought
	accessedAt time.Time
}

// ThoughtCache is a thread-safe LRU cache for recent thoughts
type ThoughtCache struct {
	mu    sync.RWMutex
	items map[string]*list.Element
	order *list.List
	size  int
}

// NewThoughtCache creates a new LRU cache with the specified size
func NewThoughtCache(size int) *ThoughtCache {
	if size <= 0 {
		size = 50
	}
	return &ThoughtCache{
		items: make(map[string]*list.Element),
		order: list.New(),
		size:  size,
	}
}

// Get retrieves a thought from the cache by ID
func (c *ThoughtCache) Get(id string) (Thought, bool) {
	c.mu.RLock()
	elem, exists := c.items[id]
	c.mu.RUnlock()

	if !exists {
		return Thought{}, false
	}

	c.mu.Lock()
	c.order.MoveToFront(elem)
	entry := elem.Value.(*thoughtCacheEntry)
	entry.accessedAt = time.Now()
	c.mu.Unlock()

	return entry.thought, true
}

// Put adds or updates a thought in the cache
func (c *ThoughtCache) Put(thought Thought) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, exists := c.items[thought.ID]; exists {
		entry := elem.Value.(*thoughtCacheEntry)
		entry.thought = thought
		entry.accessedAt = time.Now()
		c.order.MoveToFront(elem)
		return
	}

	entry := &thoughtCacheEntry{
		thought:    thought,
		accessedAt: time.Now(),
	}
	elem := c.order.PushFront(entry)
	c.items[thought.ID] = elem

	if c.order.Len() > c.size {
		c.evictOldest()
	}
}

// Remove deletes a thought from the cache by ID
func (c *ThoughtCache) Remove(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, exists := c.items[id]; exists {
		c.order.Remove(elem)
		delete(c.items, id)
	}
}

// GetAll returns all thoughts in the cache, ordered by recency (most recent first)
func (c *ThoughtCache) GetAll() []Thought {
	c.mu.RLock()
	defer c.mu.RUnlock()

	thoughts := make([]Thought, 0, c.order.Len())
	for elem := c.order.Front(); elem != nil; elem = elem.Next() {
		entry := elem.Value.(*thoughtCacheEntry)
		thoughts = append(thoughts, entry.thought)
	}
	return thoughts
}

// GetRecent returns up to 'limit' most recent thoughts from the cache
func (c *ThoughtCache) GetRecent(limit int) []Thought {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if limit <= 0 {
		limit = c.size
	}

	thoughts := make([]Thought, 0, limit)
	count := 0
	for elem := c.order.Front(); elem != nil && count < limit; elem = elem.Next() {
		entry := elem.Value.(*thoughtCacheEntry)
		thoughts = append(thoughts, entry.thought)
		count++
	}
	return thoughts
}

// Len returns the current number of items in the cache
func (c *ThoughtCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.order.Len()
}

// Clear removes all items from the cache
func (c *ThoughtCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*list.Element)
	c.order = list.New()
}

func (c *ThoughtCache) evictOldest() {
	elem := c.order.Back()
	if elem != nil {
		entry := elem.Value.(*thoughtCacheEntry)
		delete(c.items, entry.thought.ID)
		c.order.Remove(elem)
	}
}
