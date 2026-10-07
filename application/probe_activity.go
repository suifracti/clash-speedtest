package application

import (
	"sort"
	"sync"
)

// Tracks observed overlap, not proof that it caused a poor result. Mark both
// participants at admission so a shorter probe is still visible to its peer.
type probeActivity struct {
	mu     sync.Mutex
	active map[*probeOperation]bool
}
type probeOperation struct {
	kind  string
	peers map[string]bool
}

func (a *probeActivity) begin(kind string) func() []string {
	a.mu.Lock()
	if a.active == nil {
		a.active = map[*probeOperation]bool{}
	}
	op := &probeOperation{kind: kind, peers: map[string]bool{}}
	for peer := range a.active {
		op.peers[peer.kind] = true
		peer.peers[kind] = true
	}
	a.active[op] = true
	a.mu.Unlock()
	return func() []string {
		a.mu.Lock()
		defer a.mu.Unlock()
		delete(a.active, op)
		out := []string{}
		for k := range op.peers {
			out = append(out, k)
		}
		sort.Strings(out)
		return out
	}
}

func probeNetworkRunning(done <-chan struct{}) bool {
	if done == nil {
		return true
	} // Legacy/test runtimes keep conservative admission.
	select {
	case <-done:
		return false
	default:
		return true
	}
}
func finishProbeNetwork(done chan struct{}) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			if done != nil {
				close(done)
			}
		})
	}
}
func (a *probeActivity) count(kind string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	n := 0
	for op := range a.active {
		if op.kind == kind {
			n++
		}
	}
	return n
}
