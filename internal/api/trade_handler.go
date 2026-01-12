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
// @Summary 下单请求
// @Description 发送交易报单请求
// @Tags Trade
// @Accept json
// @Produce json
// @Param body body OrderRequest true "报单信息"
// @Success 202 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /trade/order [post]
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
// @Summary 获取持仓列表
// @Description 获取指定用户的当前持仓信息
// @Tags Trade
// @Accept json
// @Produce json
// @Param userID path string true "用户ID"
// @Success 200 {array} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /users/{userID}/positions [get]
func (h *TradeHandler) GetPositions(c *fiber.Ctx) error {
	userID := c.Params("userID")

	positions, err := h.tradingSvc.GetPositions(context.Background(), userID)
	if err != nil {
		return handleError(c, err)
	}

	return c.JSON(positions)
}

// GetOrders 获取订单列表
// @Summary 获取订单列表
// @Description 分页获取用户的订单（委托）记录
// @Tags Trade
// @Accept json
// @Produce json
// @Param userID path string true "用户ID"
// @Param page query int false "页码" default(1)
// @Param pageSize query int false "每页数量" default(50)
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /users/{userID}/orders [get]
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
// @Summary 同步持仓
// @Description 触发一次持仓查询请求，从CTP同步最新持仓
// @Tags Trade
// @Accept json
// @Produce json
// @Param userID path string true "用户ID"
// @Param symbol query string false "合约代码(可选)"
// @Success 202 {string} string "Accepted"
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /users/{userID}/sync-positions [post]
func (h *TradeHandler) SyncPositions(c *fiber.Ctx) error {
	userID := c.Params("userID")
	symbol := c.Query("symbol")

	if err := h.tradingSvc.QueryPositions(context.Background(), userID, symbol); err != nil {
		return handleError(c, err)
	}

	return c.SendStatus(fiber.StatusAccepted)
}

// SyncAccount 同步账户
// @Summary 同步账户资金
// @Description 触发一次账户资金查询请求，从CTP同步最新资金信息
// @Tags Trade
// @Accept json
// @Produce json
// @Param userID path string true "用户ID"
// @Success 202 {string} string "Accepted"
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /users/{userID}/sync-account [post]
func (h *TradeHandler) SyncAccount(c *fiber.Ctx) error {
	userID := c.Params("userID")

	if err := h.tradingSvc.QueryAccount(context.Background(), userID); err != nil {
		return handleError(c, err)
	}

	return c.SendStatus(fiber.StatusAccepted)
}

// CancelOrder 撤单
// @Summary 撤单
// @Description 取消尚未完全成交的订单
// @Tags Trade
// @Accept json
// @Produce json
// @Param id path int true "订单内部ID"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /trade/order/{id}/cancel [post]
func (h *TradeHandler) CancelOrder(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 32)

	if err := h.tradingSvc.CancelOrder(context.Background(), uint(id)); err != nil {
		return handleError(c, err)
	}

	return c.JSON(fiber.Map{"Message": "Cancel request sent"})
}
