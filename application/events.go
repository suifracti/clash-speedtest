package application

import "sync"

// Event represents a system or progress event transmitted to clients.
type Event struct {
	Type    string `json:"type"`
	Payload any    `json:"payload"`
}

// EventEmitter abstracts event delivery across different adapters (Wails IPC, Web SSE, CLI).
type EventEmitter interface {
	Emit(event Event)
}

// MemoryEventEmitter is an in-memory event broadcaster that implements EventEmitter.
// It is useful for test suites and headless mode.
type MemoryEventEmitter struct {
	mu        sync.RWMutex
	listeners []func(Event)
}

// NewMemoryEventEmitter creates a new in-memory event emitter.
func NewMemoryEventEmitter() *MemoryEventEmitter {
	return &MemoryEventEmitter{
		listeners: make([]func(Event), 0),
	}
}

// Emit broadcasts the event to all registered listeners.
func (m *MemoryEventEmitter) Emit(event Event) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, l := range m.listeners {
		l(event)
	}
}

// Subscribe adds an event listener callback.
func (m *MemoryEventEmitter) Subscribe(listener func(Event)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, listener)
}
