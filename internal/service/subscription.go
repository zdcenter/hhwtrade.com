package service

import (
	"context"
	"log"
	"sync"

	"gorm.io/gorm"
	"hhwtrade.com/internal/domain"
	"hhwtrade.com/internal/model"
)

// SubscriptionServiceImpl 实现 domain.SubscriptionService 接口
type SubscriptionServiceImpl struct {
	db            *gorm.DB
	marketService domain.MarketService
	notifier      domain.Notifier

	// 用于防止并发问题
	mu sync.RWMutex
}

// NewSubscriptionService 创建订阅服务
func NewSubscriptionService(
	db *gorm.DB,
	marketService domain.MarketService,
	notifier domain.Notifier,
) *SubscriptionServiceImpl {
	return &SubscriptionServiceImpl{
		db:            db,
		marketService: marketService,
		notifier:      notifier,
	}
}

// GetSubscriptions 获取订阅列表
func (s *SubscriptionServiceImpl) GetSubscriptions(ctx context.Context, page, pageSize int) ([]model.Subscription, int64, error) {
	var subs []model.Subscription
	var total int64

	// 计算偏移量
	offset := (page - 1) * pageSize

	// 统计总数
	if err := s.db.Model(&model.Subscription{}).Count(&total).Error; err != nil {
		return nil, 0, domain.NewInternalError("failed to count subscriptions", err)
	}

	// 查询数据
	if err := s.db.
		Order("sorter ASC").
		Limit(pageSize).
		Offset(offset).
		Find(&subs).Error; err != nil {
		return nil, 0, domain.NewInternalError("failed to fetch subscriptions", err)
	}

	return subs, total, nil
}

// AddSubscription 添加订阅
func (s *SubscriptionServiceImpl) AddSubscription(ctx context.Context, instrumentID, exchangeID string) (*model.Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查是否已经存在
	var count int64
	s.db.Model(&model.Subscription{}).Where("instrument_id = ?", instrumentID).Count(&count)
	if count > 0 {
		return nil, domain.NewConflictError("Subscription already exists")
	}

	sub := model.Subscription{
		InstrumentID: instrumentID,
		ExchangeID:   exchangeID,
	}

	// 1. 写入数据库
	if err := s.db.Create(&sub).Error; err != nil {
		return nil, domain.NewInternalError("failed to add subscription", err)
	}

	// 2. 触发 CTP 订阅
	if s.marketService != nil {
		if err := s.marketService.Subscribe(ctx, instrumentID); err != nil {
			log.Printf("SubscriptionService: Failed to subscribe to CTP: %v", err)
		}
	}

	log.Printf("SubscriptionService: Subscribed to %s", instrumentID)
	return &sub, nil
}

// RemoveSubscription 移除订阅
func (s *SubscriptionServiceImpl) RemoveSubscription(ctx context.Context, instrumentID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. 从数据库删除
	result := s.db.Where("instrument_id = ?", instrumentID).Delete(&model.Subscription{})
	if result.Error != nil {
		return domain.NewInternalError("failed to remove subscription", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.NewNotFoundError("subscription not found")
	}

	// 2. 触发 CTP 取消订阅
	// 只有当没有任何订阅时才取消? 这里现在是全局订阅，删了就真删了
	if s.marketService != nil {
		if err := s.marketService.Unsubscribe(ctx, instrumentID); err != nil {
			log.Printf("SubscriptionService: Failed to unsubscribe from CTP: %v", err)
		}
	}

	log.Printf("SubscriptionService: Unsubscribed from %s", instrumentID)
	return nil
}

// ReorderSubscriptions 重新排序订阅
func (s *SubscriptionServiceImpl) ReorderSubscriptions(ctx context.Context, instrumentIDs []string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		for i, symbol := range instrumentIDs {
			if err := tx.Model(&model.Subscription{}).
				Where("instrument_id = ?", symbol).
				Update("sorter", i).Error; err != nil {
				return domain.NewInternalError("failed to reorder subscriptions", err)
			}
		}
		return nil
	})
}

// RestoreSubscriptions 恢复所有已存储的订阅 (用于启动时)
func (s *SubscriptionServiceImpl) RestoreSubscriptions(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. 查找所有被订阅的合约 (去重)
	var instrumentIDs []string
	if err := s.db.Model(&model.Subscription{}).Distinct("instrument_id").Pluck("instrument_id", &instrumentIDs).Error; err != nil {
		return domain.NewInternalError("failed to fetch distinct subscriptions", err)
	}

	if len(instrumentIDs) == 0 {
		return nil
	}

	log.Printf("SubscriptionService: Restoring %d subscriptions...", len(instrumentIDs))

	// 2. 恢复 MarketService 状态
	if s.marketService != nil {
		for _, id := range instrumentIDs {
			log.Printf("SubscriptionService: Restoring %s", id)
			// 直接进入 Subscribe。
			// 因为是启动阶段，s.marketService.subscriptions 应该是空的。
			// 第一个 Subscribe(id) 会触发 isFirst=true，从而发送 Redis 命令给 ctp_main。
			if err := s.marketService.Subscribe(ctx, id); err != nil {
				log.Printf("SubscriptionService: Failed to restore CTP subscription for %s: %v", id, err)
			}
		}
	}

	return nil
}

// 确保实现了接口
var _ domain.SubscriptionService = (*SubscriptionServiceImpl)(nil)
