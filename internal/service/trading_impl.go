package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
	"hhwtrade.com/internal/domain"
	"hhwtrade.com/internal/model"
)

// TradingServiceImpl 实现 domain.TradingService 接口
type TradingServiceImpl struct {
	db        *gorm.DB
	ctpClient domain.CTPClienter
	notifier  domain.Notifier
}

// NewTradingService 创建交易服务
func NewTradingService(
	db *gorm.DB,
	ctpClient domain.CTPClienter,
	notifier domain.Notifier,
) *TradingServiceImpl {
	return &TradingServiceImpl{
		db:        db,
		ctpClient: ctpClient,
		notifier:  notifier,
	}
}

// PlaceOrder 下单
func (s *TradingServiceImpl) PlaceOrder(ctx context.Context, order *model.Order) error {
	// 1. 生成 OrderRef (如果未设置)
	if order.OrderRef == "" {
		now := time.Now()
		timestampPart := now.Unix() % 1000000
		microPart := now.Nanosecond() / 1000
		order.OrderRef = fmt.Sprintf("%06d%06d", timestampPart, microPart)
	}

	// 2. 补全交易所信息 (如果缺失)
	if order.ExchangeID == "" {
		var future model.Future
		if err := s.db.Where("instrument_id = ?", order.InstrumentID).First(&future).Error; err == nil {
			order.ExchangeID = future.ExchangeID
		}
	}

	// 3. 设置初始状态
	order.OrderStatus = model.OrderStatusSent

	// 3. 发送到 CTP (低延迟优先)
	if err := s.ctpClient.InsertOrder(ctx, order); err != nil {
		return domain.NewInternalError("failed to send order to gateway", err)
	}

	// 4. 异步写入数据库
	go func() {
		if err := s.db.Create(order).Error; err != nil {
			log.Printf("TradingService: Failed to save order %s to DB: %v", order.OrderRef, err)
		}
	}()

	log.Printf("TradingService: Order %s sent to CTP", order.OrderRef)
	return nil
}

// CancelOrder 撤单
func (s *TradingServiceImpl) CancelOrder(ctx context.Context, orderID uint) error {
	var order model.Order
	if err := s.db.First(&order, orderID).Error; err != nil {
		return domain.NewNotFoundError("order not found")
	}

	// 检查订单状态是否可撤
	if order.OrderStatus == model.OrderStatusAllTraded ||
		order.OrderStatus == model.OrderStatusCanceled ||
		order.OrderStatus == model.OrderStatusNoTradeNotQueueing {
		return &domain.AppError{
			Code:    400,
			Message: "order already in terminal state",
			Err:     domain.ErrOrderTerminal,
		}
	}

	// 发送撤单指令
	if err := s.ctpClient.CancelOrder(ctx, &order); err != nil {
		return domain.NewInternalError("failed to send cancel command", err)
	}

	log.Printf("TradingService: Cancel request sent for order %s", order.OrderRef)
	return nil
}

// QueryPositions 查询持仓 (触发 CTP 查询)
func (s *TradingServiceImpl) QueryPositions(ctx context.Context, investorID, instrumentID string) error {
	log.Printf("TradingService: Querying positions for investor %s, instrument %s", investorID, instrumentID)
	return s.ctpClient.QueryPositions(ctx, investorID, instrumentID)
}

// QueryAccount 查询账户
func (s *TradingServiceImpl) QueryAccount(ctx context.Context, investorID string) error {
	log.Printf("TradingService: Querying account for investor %s", investorID)
	return s.ctpClient.QueryAccount(ctx, investorID)
}

// GetOrders 获取订单列表
func (s *TradingServiceImpl) GetOrders(ctx context.Context, investorID string, page, pageSize int) ([]model.Order, int64, error) {
	var orders []model.Order
	var total int64

	offset := (page - 1) * pageSize
	query := s.db.Model(&model.Order{}).Where("investor_id = ?", investorID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, domain.NewInternalError("failed to count orders", err)
	}

	if err := query.Order("created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&orders).Error; err != nil {
		return nil, 0, domain.NewInternalError("failed to fetch orders", err)
	}

	return orders, total, nil
}

// GetTrades 获取成交列表
func (s *TradingServiceImpl) GetTrades(ctx context.Context, investorID string, page, pageSize int) ([]model.Trade, int64, error) {
	var trades []model.Trade
	var total int64

	offset := (page - 1) * pageSize
	query := s.db.Model(&model.Trade{}).Where("exchange_id != ''").Where("investor_id = ?", investorID) // 基于 InvestorID 过滤

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, domain.NewInternalError("failed to count trades", err)
	}

	if err := query.Order("created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&trades).Error; err != nil {
		return nil, 0, domain.NewInternalError("failed to fetch trades", err)
	}

	return trades, total, nil
}

// GetPositions 获取持仓列表
func (s *TradingServiceImpl) GetPositions(ctx context.Context, investorID string) ([]model.Position, error) {
	var positions []model.Position
	if err := s.db.Where("investor_id = ?", investorID).Find(&positions).Error; err != nil {
		return nil, domain.NewInternalError("failed to fetch positions", err)
	}
	return positions, nil
}

// 确保实现了接口
var _ domain.TradingService = (*TradingServiceImpl)(nil)
