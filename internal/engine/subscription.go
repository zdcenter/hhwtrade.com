package engine

import (
	"sync"
)

// SubscriptionState holds the state of subscriptions (e.g., symbols users are interested in).
type SubscriptionState struct {
	mu sync.RWMutex
	// activeSymbols maps symbol -> count of interested users/sessions
	activeSymbols map[string]int
}

func NewSubscriptionState() *SubscriptionState {
	return &SubscriptionState{
		activeSymbols: make(map[string]int),
	}
}

// AddSubscription increments the subscriber count for a symbol.
func (s *SubscriptionState) AddSubscription(symbol string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeSymbols[symbol]++
}

// RemoveSubscription decrements the subscriber count for a symbol.
// Returns true if the count drops to zero (meaning we can unsubscribe from upstream).
func (s *SubscriptionState) RemoveSubscription(symbol string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.activeSymbols[symbol] > 0 {
		s.activeSymbols[symbol]--
		if s.activeSymbols[symbol] == 0 {
			delete(s.activeSymbols, symbol)
			return true
		}
	}
	return false
}

// GetActiveSymbols returns a list of currently subscribed symbols.
func (s *SubscriptionState) GetActiveSymbols() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	symbols := make([]string, 0, len(s.activeSymbols))
	for sym := range s.activeSymbols {
		symbols = append(symbols, sym)
	}
	return symbols
}
