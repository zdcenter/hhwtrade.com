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

// OrderHandler 处理报单相关的请求
type OrderHandler struct {
	tradingSvc domain.TradingService
}

func NewOrderHandler(tradingSvc domain.TradingService) *OrderHandler {
	return &OrderHandler{tradingSvc: tradingSvc}
}

// OrderRequest 下单请求
type OrderRequest struct {
	InvestorID   string               `json:"InvestorID"`
	InstrumentID string               `json:"InstrumentID"`
	Direction    model.OrderDirection `json:"Direction"`
	Offset       model.OrderOffset    `json:"CombOffsetFlag"`
	Price        float64              `json:"LimitPrice"`
	Volume       int                  `json:"VolumeTotalOriginal"`
	StrategyID   *uint                `json:"StrategyID"`
}

// InsertOrder 下单
func (h *OrderHandler) InsertOrder(c *fiber.Ctx) error {
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
		InvestorID:          req.InvestorID,
		InstrumentID:        req.InstrumentID,
		Direction:           req.Direction,
		CombOffsetFlag:      req.Offset,
		LimitPrice:          req.Price,
		VolumeTotalOriginal: req.Volume,
		OrderRef:            orderRef,
		StrategyID:          req.StrategyID,
	}

	if err := h.tradingSvc.PlaceOrder(context.Background(), order); err != nil {
		return handleError(c, err)
	}

	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"Status":    true,
		"Message":   "Order sent",
		"OrderRef":  orderRef,
		"RequestID": orderRef,
	})
}

// GetOrders 获取订单列表
func (h *OrderHandler) GetOrders(c *fiber.Ctx) error {
	investorID := c.Params("investorID")
	page, _ := strconv.Atoi(c.Query("page", "1"))
	pageSize, _ := strconv.Atoi(c.Query("pageSize", "50"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}

	orders, total, err := h.tradingSvc.GetOrders(context.Background(), investorID, page, pageSize)
	if err != nil {
		return handleError(c, err)
	}

	return c.JSON(fiber.Map{
		"Data": orders,
		"Pagination": fiber.Map{
			"Page":     page,
			"PageSize": pageSize,
			"Total":    total,
		},
	})
}

// CancelOrder 撤单
func (h *OrderHandler) CancelOrder(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 32)
	if err := h.tradingSvc.CancelOrder(context.Background(), uint(id)); err != nil {
		return handleError(c, err)
	}
	return c.JSON(fiber.Map{"Message": "Cancel request sent"})
}
