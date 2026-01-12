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
// @Summary 获取订阅列表
// @Description 分页获取当前系统的所有行情订阅信息
// @Tags Subscriptions
// @Accept json
// @Produce json
// @Param page query int false "页码" default(1)
// @Param pageSize query int false "每页数量" default(10)
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /subscriptions [get]
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

// AddSubscriptionRequest 添加订阅请求参数
type AddSubscriptionRequest struct {
	InstrumentID string `json:"InstrumentID"`
	ExchangeID   string `json:"ExchangeID"`
}

// AddSubscription 添加订阅
// @Summary 添加订阅
// @Description 添加一个新的行情订阅请求
// @Tags Subscriptions
// @Accept json
// @Produce json
// @Param body body AddSubscriptionRequest true "订阅信息"
// @Success 201 {object} model.Subscription
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /subscriptions [post]
func (h *SubscriptionHandler) AddSubscription(c *fiber.Ctx) error {
	var req AddSubscriptionRequest

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
// @Summary 移除订阅
// @Description 根据合约ID移除行情订阅
// @Tags Subscriptions
// @Accept json
// @Produce json
// @Param symbol path string true "合约代码 (InstrumentID)"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /subscriptions/{symbol} [delete]
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

// ReorderSubscriptionsRequest 排序订阅请求参数
type ReorderSubscriptionsRequest struct {
	InstrumentIDs []string `json:"InstrumentIDs"`
}

// ReorderSubscriptions 重新排序订阅
// @Summary 重新排序订阅
// @Description 批量更新订阅列表的顺序，主要用于前端展示
// @Tags Subscriptions
// @Accept json
// @Produce json
// @Param body body ReorderSubscriptionsRequest true "排序后的合约ID列表"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /subscriptions/reorder [put]
func (h *SubscriptionHandler) ReorderSubscriptions(c *fiber.Ctx) error {
	var req ReorderSubscriptionsRequest

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"Error": "Invalid request body"})
	}

	err := h.subscriptionSvc.ReorderSubscriptions(context.Background(), req.InstrumentIDs)
	if err != nil {
		return handleError(c, err)
	}

	return c.JSON(fiber.Map{"Status": true})
}
