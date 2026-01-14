package service

import (
	"context"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"
	"hhwtrade.com/internal/domain"
)

// MarketServiceImpl 实现 domain.MarketService 接口
type MarketServiceImpl struct {
	ctpClient domain.CTPClienter
	notifier  domain.Notifier

	// 订阅引用计数
	subscriptions map[string]int
	mu            sync.RWMutex

	db *gorm.DB
}

// NewMarketService 创建行情服务
func NewMarketService(ctpClient domain.CTPClienter, notifier domain.Notifier, db *gorm.DB) *MarketServiceImpl {
	return &MarketServiceImpl{
		ctpClient:     ctpClient,
		notifier:      notifier,
		db:            db,
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

		// 检查并在必要时查询合约费率
		go s.checkAndQueryRates(context.Background(), instrumentID)
	}

	return nil
}

func (s *MarketServiceImpl) checkAndQueryRates(ctx context.Context, instrumentID string) {
	// 使用Raw SQL或模型查询以避免循环依赖问题(如果Model引用有问题)，这里Model是安全的
	// 简单的查询 LongMarginRatioByMoney 字段
	type RateCheck struct {
		LongMarginRatioByMoney float64
	}
	var res RateCheck

	// 只查需要的字段
	if err := s.db.Table("future_futures").Select("long_margin_ratio_by_money").Where("instrument_id = ?", instrumentID).Scan(&res).Error; err == nil {
		// 如果费率是 0 (默认值)，则认为需要查询
		// 注意：有些合约可能真的费率为0？不太可能对所有字段都是0。
		// 这里只把 LongMarginRatioByMoney 作为 flag
		if res.LongMarginRatioByMoney == 0 {
			log.Printf("MarketService: Rate info missing for %s, checking CTP...", instrumentID)
			s.RequestRateUpdate(ctx, instrumentID)
		}
	}
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

// QueryMarginRate 查询合约保证金率
func (s *MarketServiceImpl) QueryMarginRate(ctx context.Context, instrumentID string) error {
	log.Printf("MarketService: Querying margin rate for %s", instrumentID)
	return s.ctpClient.QueryMarginRate(ctx, instrumentID)
}

// QueryCommissionRate 查询合约手续费率
func (s *MarketServiceImpl) QueryCommissionRate(ctx context.Context, instrumentID string) error {
	log.Printf("MarketService: Querying commission rate for %s", instrumentID)
	return s.ctpClient.QueryCommissionRate(ctx, instrumentID)
}

// RequestRateUpdate 请求更新所有费率 (带流控处理)
func (s *MarketServiceImpl) RequestRateUpdate(ctx context.Context, instrumentID string) error {
	// 1. 查询保证金率
	if err := s.ctpClient.QueryMarginRate(ctx, instrumentID); err != nil {
		return err
	}

	// 2. 必须等待，否则 CTP 会返回 -2 (查询太频繁)
	// 通常 CTP 限制为 1次/秒
	time.Sleep(1200 * time.Millisecond)

	// 3. 查询手续费率
	if err := s.ctpClient.QueryCommissionRate(ctx, instrumentID); err != nil {
		return err
	}

	return nil
}

// 确保实现了接口
var _ domain.MarketService = (*MarketServiceImpl)(nil)
