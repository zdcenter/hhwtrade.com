package service

import (
	"context"
	"encoding/json"
	"log"

	"gorm.io/gorm"
	"hhwtrade.com/internal/domain"
	"hhwtrade.com/internal/model"
	"hhwtrade.com/internal/strategies"
)

// StrategyServiceImpl 实现 domain.StrategyService 接口
type StrategyServiceImpl struct {
	db             *gorm.DB
	executor       *strategies.Executor
	tradingService domain.TradingService
}

// NewStrategyService 创建策略服务
func NewStrategyService(
	db *gorm.DB,
	executor *strategies.Executor,
	tradingService domain.TradingService,
) *StrategyServiceImpl {
	return &StrategyServiceImpl{
		db:             db,
		executor:       executor,
		tradingService: tradingService,
	}
}

// LoadActiveStrategies 加载活跃策略
func (s *StrategyServiceImpl) LoadActiveStrategies() {
	log.Println("StrategyService: Loading active strategies...")
	s.executor.LoadActiveStrategies()
}

// GetActiveSymbols 获取策略监控的合约列表
func (s *StrategyServiceImpl) GetActiveSymbols() []string {
	return s.executor.GetSymbols()
}

// CreateStrategy 创建策略
func (s *StrategyServiceImpl) CreateStrategy(ctx context.Context, strategy *model.Strategy) error {
	if err := s.db.Create(strategy).Error; err != nil {
		return domain.NewInternalError("failed to create strategy", err)
	}

	log.Printf("StrategyService: Strategy created: %d", strategy.ID)

	// 重新加载策略
	s.executor.Reload()
	return nil
}

// StopStrategy 停止策略
func (s *StrategyServiceImpl) StopStrategy(ctx context.Context, strategyID uint) error {
	result := s.db.Model(&model.Strategy{}).
		Where("id = ?", strategyID).
		Update("status", model.StrategyStatusStopped)

	if result.Error != nil {
		return domain.NewInternalError("failed to stop strategy", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.NewNotFoundError("strategy not found")
	}

	log.Printf("StrategyService: Strategy stopped: %d", strategyID)
	s.executor.Reload()
	return nil
}

// StartStrategy 启动策略
func (s *StrategyServiceImpl) StartStrategy(ctx context.Context, strategyID uint) error {
	result := s.db.Model(&model.Strategy{}).
		Where("id = ?", strategyID).
		Update("status", model.StrategyStatusActive)

	if result.Error != nil {
		return domain.NewInternalError("failed to start strategy", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.NewNotFoundError("strategy not found")
	}

	log.Printf("StrategyService: Strategy started: %d", strategyID)
	s.executor.Reload()
	return nil
}

// GetStrategies 获取用户策略列表
func (s *StrategyServiceImpl) GetStrategies(ctx context.Context, userID string, page, pageSize int) ([]model.Strategy, int64, error) {
	var strategies []model.Strategy
	var total int64

	offset := (page - 1) * pageSize

	query := s.db.Model(&model.Strategy{}).Where("user_id = ?", userID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, domain.NewInternalError("failed to count strategies", err)
	}

	if err := query.Order("id DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&strategies).Error; err != nil {
		return nil, 0, domain.NewInternalError("failed to fetch strategies", err)
	}

	return strategies, total, nil
}

// GetStrategy 获取策略详情
func (s *StrategyServiceImpl) GetStrategy(ctx context.Context, strategyID uint) (*model.Strategy, error) {
	var strategy model.Strategy
	if err := s.db.First(&strategy, strategyID).Error; err != nil {
		return nil, domain.NewNotFoundError("strategy not found")
	}
	return &strategy, nil
}

// UpdateStrategy 更新策略
func (s *StrategyServiceImpl) UpdateStrategy(ctx context.Context, strategyID uint, updates map[string]interface{}) error {
	result := s.db.Model(&model.Strategy{}).Where("id = ?", strategyID).Updates(updates)
	if result.Error != nil {
		return domain.NewInternalError("failed to update strategy", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.NewNotFoundError("strategy not found")
	}

	s.executor.Reload()
	return nil
}

// DeleteStrategy 删除策略
func (s *StrategyServiceImpl) DeleteStrategy(ctx context.Context, strategyID uint) error {
	result := s.db.Delete(&model.Strategy{}, strategyID)
	if result.Error != nil {
		return domain.NewInternalError("failed to delete strategy", result.Error)
	}
	if result.RowsAffected == 0 {
		return domain.NewNotFoundError("strategy not found")
	}

	s.executor.Reload()
	return nil
}

// Reload 重新加载策略
func (s *StrategyServiceImpl) Reload() {
	log.Println("StrategyService: Reloading strategies...")
	s.executor.Reload()
}

// OnMarketData 处理行情数据 (由 Engine 调用)
func (s *StrategyServiceImpl) OnMarketData(ctx context.Context, symbol string, price float64) {
	orders := s.executor.OnMarketData(symbol, price)

	for _, order := range orders {
		if err := s.tradingService.PlaceOrder(ctx, order); err != nil {
			log.Printf("StrategyService: Failed to place order: %v", err)
			continue
		}
		log.Printf("StrategyService: Strategy triggered order for %s at price %.2f", symbol, price)
	}
}

// CreateStrategyFromRequest 从请求创建策略
func (s *StrategyServiceImpl) CreateStrategyFromRequest(ctx context.Context, userID, instrumentID string, strategyType model.StrategyType, config json.RawMessage) (*model.Strategy, error) {
	strategy := model.Strategy{
		UserID:       userID,
		InstrumentID: instrumentID,
		Type:         strategyType,
		Status:       model.StrategyStatusActive,
		Config:       config,
	}

	if err := s.CreateStrategy(ctx, &strategy); err != nil {
		return nil, err
	}

	return &strategy, nil
}

// 确保实现了接口
var _ domain.StrategyService = (*StrategyServiceImpl)(nil)
