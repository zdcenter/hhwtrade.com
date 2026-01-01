package service

import (
	"context"
	"log"
	"sync"

	"hhwtrade.com/internal/domain"
)

// MarketServiceImpl 实现 domain.MarketService 接口
type MarketServiceImpl struct {
	ctpClient domain.CTPClient
	notifier  domain.Notifier

	// 订阅引用计数
	subscriptions map[string]int
	mu            sync.RWMutex
}

// NewMarketService 创建行情服务
func NewMarketService(ctpClient domain.CTPClient, notifier domain.Notifier) *MarketServiceImpl {
	return &MarketServiceImpl{
		ctpClient:     ctpClient,
		notifier:      notifier,
		subscriptions: make(map[string]int),
	}
}

// Subscribe 订阅合约行情
func (s *MarketServiceImpl) Subscribe(ctx context.Context, instrumentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.subscriptions[instrumentID]++
	isFirst := s.subscriptions[instrumentID] == 1

	if isFirst {
		log.Printf("MarketService: First subscription for %s, sending to CTP", instrumentID)
		if err := s.ctpClient.Subscribe(ctx, instrumentID); err != nil {
			s.subscriptions[instrumentID]--
			return domain.NewInternalError("failed to subscribe", err)
		}
	}

	return nil
}

// Unsubscribe 取消订阅合约行情
func (s *MarketServiceImpl) Unsubscribe(ctx context.Context, instrumentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.subscriptions[instrumentID] > 0 {
		s.subscriptions[instrumentID]--

		if s.subscriptions[instrumentID] == 0 {
			log.Printf("MarketService: No more subscribers for %s, unsubscribing from CTP", instrumentID)
			delete(s.subscriptions, instrumentID)

			if err := s.ctpClient.Unsubscribe(ctx, instrumentID); err != nil {
				return domain.NewInternalError("failed to unsubscribe", err)
			}
		}
	}

	return nil
}

// GetActiveSymbols 获取当前活跃的订阅合约
func (s *MarketServiceImpl) GetActiveSymbols() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	symbols := make([]string, 0, len(s.subscriptions))
	for symbol := range s.subscriptions {
		symbols = append(symbols, symbol)
	}
	return symbols
}

// SyncInstruments 同步合约信息
func (s *MarketServiceImpl) SyncInstruments(ctx context.Context) error {
	log.Println("MarketService: Triggering instrument sync from CTP")
	return s.ctpClient.SyncInstruments(ctx)
}

// AddExistingSubscription 添加已存在的订阅（用于启动时恢复）
func (s *MarketServiceImpl) AddExistingSubscription(instrumentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscriptions[instrumentID]++
	s.subscriptions[instrumentID]++
}

// ResubscribeAll 重新订阅所有活跃合约
func (s *MarketServiceImpl) ResubscribeAll(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	log.Printf("MarketService: Resubscribing to %d instruments...", len(s.subscriptions))

	for instrumentID, count := range s.subscriptions {
		if count > 0 {
			log.Printf("MarketService: Re-subscribing to %s", instrumentID)
			if err := s.ctpClient.Subscribe(ctx, instrumentID); err != nil {
				log.Printf("MarketService: Failed to re-subscribe to %s: %v", instrumentID, err)
				// Continue with other subscriptions even if one fails
			}
		}
	}
	return nil
}

// 确保实现了接口
var _ domain.MarketService = (*MarketServiceImpl)(nil)
