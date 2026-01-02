package api

import (
	"context"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"hhwtrade.com/internal/domain"
)

// SubscriptionHandler 处理订阅相关的 HTTP 请求
type SubscriptionHandler struct {
	subscriptionSvc domain.SubscriptionService
}

// NewSubscriptionHandler 创建订阅处理器
func NewSubscriptionHandler(subscriptionSvc domain.SubscriptionService) *SubscriptionHandler {
	return &SubscriptionHandler{subscriptionSvc: subscriptionSvc}
}

// GetSubscriptions 获取订阅列表
// GET /api/subscriptions?page=1&pageSize=10
func (h *SubscriptionHandler) GetSubscriptions(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	pageSize, _ := strconv.Atoi(c.Query("pageSize", "10"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	subs, total, err := h.subscriptionSvc.GetSubscriptions(context.Background(), page, pageSize)
	if err != nil {
		return handleError(c, err)
	}

	return SendPaginatedResponse(c, subs, page, pageSize, total)
}

// AddSubscription 添加订阅
// POST /api/subscriptions
func (h *SubscriptionHandler) AddSubscription(c *fiber.Ctx) error {
	var req struct {
		InstrumentID string `json:"InstrumentID"`
		ExchangeID   string `json:"ExchangeID"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"Error": "Invalid request body"})
	}

	sub, err := h.subscriptionSvc.AddSubscription(context.Background(), req.InstrumentID, req.ExchangeID)
	if err != nil {
		return handleError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(sub)
}

// RemoveSubscription 移除订阅
// DELETE /api/subscriptions/:symbol
func (h *SubscriptionHandler) RemoveSubscription(c *fiber.Ctx) error {
	instrumentID := c.Params("symbol")

	err := h.subscriptionSvc.RemoveSubscription(context.Background(), instrumentID)
	if err != nil {
		return handleError(c, err)
	}

	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"Status":       true,
		"Message":      "Unsubscribed successfully",
		"InstrumentID": instrumentID,
	})
}

// ReorderSubscriptions 重新排序订阅
// PUT /api/subscriptions/reorder
func (h *SubscriptionHandler) ReorderSubscriptions(c *fiber.Ctx) error {
	var req struct {
		InstrumentIDs []string `json:"InstrumentIDs"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"Error": "Invalid request body"})
	}

	err := h.subscriptionSvc.ReorderSubscriptions(context.Background(), req.InstrumentIDs)
	if err != nil {
		return handleError(c, err)
	}

	return c.JSON(fiber.Map{"Status": true})
}
