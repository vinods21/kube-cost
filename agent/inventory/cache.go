package inventory

import (
	"fmt"
	"sync"
)

type Cache struct {
	mu      sync.RWMutex
	entries map[string]string
}

func NewCache() *Cache {
	return &Cache{entries: make(map[string]string)}
}

func (c *Cache) Changed(event Event) (bool, string, error) {
	if err := event.Validate(); err != nil {
		return false, "", err
	}
	fingerprint, err := Fingerprint(event.Observation)
	if err != nil {
		return false, "", err
	}

	c.mu.RLock()
	current, exists := c.entries[event.Key]
	c.mu.RUnlock()
	return !exists || current != fingerprint, fingerprint, nil
}

func (c *Cache) Exists(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, exists := c.entries[key]
	return exists
}

func (c *Cache) Commit(key, fingerprint string) error {
	if key == "" || fingerprint == "" {
		return fmt.Errorf("cache key and fingerprint are required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[key] = fingerprint
	return nil
}

func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

func (c *Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}
