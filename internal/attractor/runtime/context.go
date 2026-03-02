// Package runtime holds mutable execution state for Attractor pipelines.
package runtime

import "sync"

// Context is a thread-safe key-value store for pipeline execution state.
type Context struct {
	mu   sync.RWMutex
	data map[string]string
}

// NewContext creates an empty Context.
func NewContext() *Context {
	return &Context{data: make(map[string]string)}
}

// Get returns the value for key and whether it exists.
func (c *Context) Get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.data[key]
	return v, ok
}

// Set stores a key-value pair.
func (c *Context) Set(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = value
}

// Delete removes a key.
func (c *Context) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, key)
}

// Merge copies all key-value pairs from other into this context.
// Existing keys are overwritten.
func (c *Context) Merge(other map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, v := range other {
		c.data[k] = v
	}
}

// Snapshot returns a copy of all key-value pairs.
func (c *Context) Snapshot() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	snap := make(map[string]string, len(c.data))
	for k, v := range c.data {
		snap[k] = v
	}
	return snap
}

// Clone returns a deep copy of the context.
func (c *Context) Clone() *Context {
	return &Context{data: c.Snapshot()}
}

// Len returns the number of entries.
func (c *Context) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.data)
}
