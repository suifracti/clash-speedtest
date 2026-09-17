package desktop

import (
	"context"
	"sync"

	"github.com/faceair/clash-speedtest/application"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// WailsEventEmitter emits events to the frontend via Wails runtime.EventsEmit.
type WailsEventEmitter struct {
	mu  sync.RWMutex
	ctx context.Context
}

// NewWailsEventEmitter creates an event emitter for desktop Wails.
func NewWailsEventEmitter() *WailsEventEmitter {
	return &WailsEventEmitter{}
}

// SetContext sets the active Wails application context.
func (e *WailsEventEmitter) SetContext(ctx context.Context) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ctx = ctx
}

// Emit pushes the event to the frontend via Wails event bus.
func (e *WailsEventEmitter) Emit(event application.Event) {
	e.mu.RLock()
	ctx := e.ctx
	e.mu.RUnlock()

	if ctx != nil && ctx.Value("events") != nil {
		defer func() {
			_ = recover()
		}()
		runtime.EventsEmit(ctx, event.Type, event.Payload)
	}
}
