package realtime

import "sync"

type DirectHub struct {
	mu          sync.RWMutex
	subscribers map[int64]map[chan any]struct{}
}

func NewDirectHub() *DirectHub {
	return &DirectHub{subscribers: make(map[int64]map[chan any]struct{})}
}

func (h *DirectHub) Subscribe(conversationID int64) (<-chan any, func()) {
	channel := make(chan any, 32)
	if h == nil || conversationID <= 0 {
		close(channel)
		return channel, func() {}
	}
	h.mu.Lock()
	if h.subscribers[conversationID] == nil {
		h.subscribers[conversationID] = make(map[chan any]struct{})
	}
	h.subscribers[conversationID][channel] = struct{}{}
	h.mu.Unlock()
	var once sync.Once
	return channel, func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subscribers[conversationID], channel)
			if len(h.subscribers[conversationID]) == 0 {
				delete(h.subscribers, conversationID)
			}
			close(channel)
			h.mu.Unlock()
		})
	}
}

func (h *DirectHub) Publish(conversationID int64, event any) {
	if h == nil || conversationID <= 0 {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for subscriber := range h.subscribers[conversationID] {
		select {
		case subscriber <- event:
		default:
			// The client catches up from its last durable sequence after reconnect.
		}
	}
}
