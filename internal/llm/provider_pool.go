package llm

import "sync"

// ProviderPool caches LLM clients keyed by (baseURL, apiKey) pair.
// It is safe for concurrent use.
type ProviderPool struct {
	mu      sync.RWMutex
	clients map[string]*Client
}

// NewProviderPool creates an empty ProviderPool.
func NewProviderPool() *ProviderPool {
	return &ProviderPool{
		clients: make(map[string]*Client),
	}
}

// Get returns an existing Client for the given credentials or creates a new one.
func (p *ProviderPool) Get(baseURL, apiKey string) *Client {
	key := baseURL + "\x00" + apiKey

	p.mu.RLock()
	c, ok := p.clients[key]
	p.mu.RUnlock()
	if ok {
		return c
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	// Double-check after acquiring write lock.
	if c, ok = p.clients[key]; ok {
		return c
	}
	c = NewClient(baseURL, apiKey)
	p.clients[key] = c
	return c
}
