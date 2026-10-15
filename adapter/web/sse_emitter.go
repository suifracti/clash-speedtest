package web

import (
	"encoding/json"
	"sync"

	"github.com/faceair/clash-speedtest/application"
)

// SSEEmitter implements application.EventEmitter and manages Server-Sent Events subscribers.
type SSEEmitter struct {
	clients map[chan []byte]struct{}
	mu      sync.Mutex
}

// NewSSEEmitter creates a new SSE broadcaster.
func NewSSEEmitter() *SSEEmitter {
	return &SSEEmitter{
		clients: make(map[chan []byte]struct{}),
	}
}

// Subscribe returns a new channel that receives raw JSON event payloads.
func (b *SSEEmitter) Subscribe() chan []byte {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.clients == nil {
		b.clients = make(map[chan []byte]struct{})
	}
	ch := make(chan []byte, 64)
	b.clients[ch] = struct{}{}
	return ch
}

// Unsubscribe removes and closes a client channel safely.
func (b *SSEEmitter) Unsubscribe(ch chan []byte) {
	if b == nil || ch == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.clients != nil {
		if _, ok := b.clients[ch]; ok {
			delete(b.clients, ch)
			close(ch)
		}
	}
}

// Emit broadcasts an event to all connected SSE clients.
func (b *SSEEmitter) Emit(event application.Event) {
	if b == nil {
		return
	}
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients {
		select {
		case ch <- data:
		default:
		}
	}
}
