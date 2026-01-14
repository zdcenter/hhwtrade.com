package api

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"hhwtrade.com/internal/domain"
)

// PositionHandler 处理持仓和账户相关的请求
type PositionHandler struct {
	tradingSvc domain.TradingService
}

func NewPositionHandler(tradingSvc domain.TradingService) *PositionHandler {
	return &PositionHandler{tradingSvc: tradingSvc}
}

// GetPositions 获取持仓列表
func (h *PositionHandler) GetPositions(c *fiber.Ctx) error {
	investorID := c.Params("investorID")
	positions, err := h.tradingSvc.GetPositions(context.Background(), investorID)
	if err != nil {
		return handleError(c, err)
	}
	return c.JSON(fiber.Map{
		"Data": positions,
	})
}

// SyncPositions 同步持仓 (从 CTP 查询)
func (h *PositionHandler) SyncPositions(c *fiber.Ctx) error {
	investorID := c.Params("investorID")
	symbol := c.Query("symbol")
	if err := h.tradingSvc.QueryPositions(context.Background(), investorID, symbol); err != nil {
		return handleError(c, err)
	}
	return c.SendStatus(fiber.StatusAccepted)
}

// SyncAccount 同步账户
func (h *PositionHandler) SyncAccount(c *fiber.Ctx) error {
	userID := c.Params("userID")
	if err := h.tradingSvc.QueryAccount(context.Background(), userID); err != nil {
		return handleError(c, err)
	}
	return c.SendStatus(fiber.StatusAccepted)
}
