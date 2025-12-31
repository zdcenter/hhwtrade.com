package api

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"hhwtrade.com/internal/domain"
	"hhwtrade.com/internal/model"
)

// TradeHandler 处理交易相关的 HTTP 请求
type TradeHandler struct {
	tradingSvc domain.TradingService
}

// NewTradeHandler 创建交易处理器
func NewTradeHandler(tradingSvc domain.TradingService) *TradeHandler {
	return &TradeHandler{tradingSvc: tradingSvc}
}

// OrderRequest 下单请求
type OrderRequest struct {
	UserID       string               `json:"UserID"`
	InstrumentID string               `json:"InstrumentID"`
	Direction    model.OrderDirection `json:"Direction"`
	Offset       model.OrderOffset    `json:"CombOffsetFlag"`
	Price        float64              `json:"LimitPrice"`
	Volume       int                  `json:"VolumeTotalOriginal"`
	StrategyID   *uint                `json:"StrategyID"`
}

// InsertOrder 下单
// POST /api/trade/order
func (h *TradeHandler) InsertOrder(c *fiber.Ctx) error {
	var req OrderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"Error": "Invalid request body"})
	}

	// 生成唯一 OrderRef
	now := time.Now()
	timestampPart := now.Unix() % 1000000
	microPart := now.Nanosecond() / 1000
	orderRef := fmt.Sprintf("%06d%06d", timestampPart, microPart)

	order := &model.Order{
		UserID:              req.UserID,
		InstrumentID:        req.InstrumentID,
		OrderRef:            orderRef,
		Direction:           req.Direction,
		CombOffsetFlag:      req.Offset,
		LimitPrice:          req.Price,
		VolumeTotalOriginal: req.Volume,
		StrategyID:          req.StrategyID,
	}

	if err := h.tradingSvc.PlaceOrder(context.Background(), order); err != nil {
		return handleError(c, err)
	}

	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"Message":   "Order sent",
		"OrderRef":  orderRef,
		"RequestID": orderRef,
	})
}

// GetPositions 获取持仓列表
// GET /api/users/:userID/positions
func (h *TradeHandler) GetPositions(c *fiber.Ctx) error {
	userID := c.Params("userID")

	positions, err := h.tradingSvc.GetPositions(context.Background(), userID)
	if err != nil {
		return handleError(c, err)
	}

	return c.JSON(positions)
}

// GetOrders 获取订单列表
// GET /api/users/:userID/orders
func (h *TradeHandler) GetOrders(c *fiber.Ctx) error {
	userID := c.Params("userID")
	page, _ := strconv.Atoi(c.Query("page", "1"))
	pageSize, _ := strconv.Atoi(c.Query("pageSize", "50"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}

	orders, total, err := h.tradingSvc.GetOrders(context.Background(), userID, page, pageSize)
	if err != nil {
		return handleError(c, err)
	}

	return SendPaginatedResponse(c, orders, page, pageSize, total)
}

// SyncPositions 同步持仓
// POST /api/users/:userID/sync-positions
func (h *TradeHandler) SyncPositions(c *fiber.Ctx) error {
	userID := c.Params("userID")
	symbol := c.Query("symbol")

	if err := h.tradingSvc.QueryPositions(context.Background(), userID, symbol); err != nil {
		return handleError(c, err)
	}

	return c.SendStatus(fiber.StatusAccepted)
}

// SyncAccount 同步账户
// POST /api/users/:userID/sync-account
func (h *TradeHandler) SyncAccount(c *fiber.Ctx) error {
	userID := c.Params("userID")

	if err := h.tradingSvc.QueryAccount(context.Background(), userID); err != nil {
		return handleError(c, err)
	}

	return c.SendStatus(fiber.StatusAccepted)
}

// CancelOrder 撤单
// POST /api/trade/order/:id/cancel
func (h *TradeHandler) CancelOrder(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 32)

	if err := h.tradingSvc.CancelOrder(context.Background(), uint(id)); err != nil {
		return handleError(c, err)
	}

	return c.JSON(fiber.Map{"Message": "Cancel request sent"})
}
