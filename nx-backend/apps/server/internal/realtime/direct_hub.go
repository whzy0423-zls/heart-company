package realtime

import "sync"

type DirectHub struct {
	mu                      sync.RWMutex
	conversationSubscribers map[int64]map[chan any]struct{}
	userSubscribers         map[int64]map[chan any]struct{}
}

func NewDirectHub() *DirectHub {
	return &DirectHub{
		conversationSubscribers: make(map[int64]map[chan any]struct{}),
		userSubscribers:         make(map[int64]map[chan any]struct{}),
	}
}

func (h *DirectHub) Subscribe(conversationID int64) (<-chan any, func()) {
	if h == nil {
		return closedDirectSubscription()
	}
	return h.subscribe(h.conversationSubscribers, conversationID)
}

func (h *DirectHub) SubscribeUser(userID int64) (<-chan any, func()) {
	if h == nil {
		return closedDirectSubscription()
	}
	return h.subscribe(h.userSubscribers, userID)
}

func (h *DirectHub) Publish(conversationID int64, event any) {
	if h == nil {
		return
	}
	h.publish(h.conversationSubscribers, conversationID, event)
}

func (h *DirectHub) PublishUser(userID int64, event any) {
	if h == nil {
		return
	}
	h.publish(h.userSubscribers, userID, event)
}

func closedDirectSubscription() (<-chan any, func()) {
	channel := make(chan any)
	close(channel)
	return channel, func() {}
}

func (h *DirectHub) subscribe(subscribers map[int64]map[chan any]struct{}, key int64) (<-chan any, func()) {
	channel := make(chan any, 32)
	if h == nil || key <= 0 {
		close(channel)
		return channel, func() {}
	}
	h.mu.Lock()
	if subscribers[key] == nil {
		subscribers[key] = make(map[chan any]struct{})
	}
	subscribers[key][channel] = struct{}{}
	h.mu.Unlock()
	var once sync.Once
	return channel, func() {
		once.Do(func() {
			h.mu.Lock()
			delete(subscribers[key], channel)
			if len(subscribers[key]) == 0 {
				delete(subscribers, key)
			}
			close(channel)
			h.mu.Unlock()
		})
	}
}

func (h *DirectHub) publish(subscribers map[int64]map[chan any]struct{}, key int64, event any) {
	if h == nil || key <= 0 {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for subscriber := range subscribers[key] {
		select {
		case subscriber <- event:
		default:
			// The client catches up from its last durable sequence after reconnect.
		}
	}
}
