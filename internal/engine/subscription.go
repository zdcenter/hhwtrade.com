package engine

import (
	"sync"
)

// SubscriptionState 保存订阅状态（例如用户感兴趣的合约）
type SubscriptionState struct {
	mu sync.RWMutex
	// activeSymbols 映射 symbol -> 感兴趣的用户/会话数量
	activeSymbols map[string]int
}

func NewSubscriptionState() *SubscriptionState {
	return &SubscriptionState{
		activeSymbols: make(map[string]int),
	}
}

// AddSubscription 增加合约的订阅者计数
// 如果这是该合约的第一个订阅，则返回 true
func (s *SubscriptionState) AddSubscription(symbol string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeSymbols[symbol]++
	return s.activeSymbols[symbol] == 1
}

// RemoveSubscription 减少合约的订阅者计数
// 如果计数降至零（意味着我们可以从上游取消订阅），则返回 true
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

// GetActiveSymbols 返回当前订阅的合约列表
func (s *SubscriptionState) GetActiveSymbols() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	symbols := make([]string, 0, len(s.activeSymbols))
	for sym := range s.activeSymbols {
		symbols = append(symbols, sym)
	}
	return symbols
}
