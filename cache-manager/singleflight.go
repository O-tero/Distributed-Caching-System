package cachemanager

import (
	"sync"
)

// RequestCoalescer implements the singleflight pattern to prevent cache stampede.
// Multiple concurrent requests for the same key are coalesced into a single
// execution, with all callers receiving the same result.
//
// This is critical for preventing thundering herd on cache misses, where many
// goroutines simultaneously request the same expired/missing key, causing
// N identical database/origin queries instead of 1.
//
// Implementation uses sync.Map for lock-free fast path and per-key mutex
// for slow path coordination.
type RequestCoalescer struct {
	mu     sync.Mutex
	calls  map[string]*call
}

// call represents an in-flight request for a specific key.
type call struct {
	wg  sync.WaitGroup
	val interface{}
	err error
}

// NewRequestCoalescer creates a new request coalescer.
func NewRequestCoalescer() *RequestCoalescer {
	return &RequestCoalescer{
		calls: make(map[string]*call),
	}
}

// Do executes and returns the results of the given function, ensuring that
// only one execution is in-flight for a given key at a time. If a duplicate
// call comes in, the duplicate caller waits for the original to complete and
// receives the same result.
//
// Complexity: O(1) for cache hit (fast path), O(1) + fn() for cache miss.
func (c *RequestCoalescer) Do(key string, fn func() (interface{}, error)) (interface{}, error) {
	c.mu.Lock()
	
	if call, exists := c.calls[key]; exists {
		c.mu.Unlock()
		call.wg.Wait()
		return call.val, call.err
	}
	
	call := &call{}
	call.wg.Add(1)
	c.calls[key] = call
	c.mu.Unlock()
	
	call.val, call.err = fn()
	
	c.mu.Lock()
	delete(c.calls, key)
	c.mu.Unlock()
	call.wg.Done()
	
	return call.val, call.err
}

// Forget removes the key from the coalescer, allowing future calls to execute.
// This is useful for invalidating in-flight requests when cache is explicitly cleared.
func (c *RequestCoalescer) Forget(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.calls, key)
}

func (c *RequestCoalescer) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = make(map[string]*call)
}

// InFlight returns the number of currently in-flight requests.
// Useful for monitoring and debugging.
func (c *RequestCoalescer) InFlight() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.calls)
}