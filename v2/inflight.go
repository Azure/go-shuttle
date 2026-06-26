package shuttle

import "sync"

type inFlightHandlers struct {
	mu      sync.Mutex
	active  int
	drained chan struct{}
}

func newInFlightHandlers() *inFlightHandlers {
	drained := make(chan struct{})
	close(drained)
	return &inFlightHandlers{
		drained: drained,
	}
}

func (h *inFlightHandlers) add() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.active == 0 {
		h.drained = make(chan struct{})
	}
	h.active++
}

func (h *inFlightHandlers) done() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.active--
	if h.active == 0 {
		close(h.drained)
	}
}

func (h *inFlightHandlers) doneCh() <-chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.drained
}

func (h *inFlightHandlers) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.active
}
