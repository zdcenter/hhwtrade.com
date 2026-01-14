package api

import (
	"context"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"hhwtrade.com/internal/domain"
)

// TradeHandler 处理成交记录相关的请求
type TradeHandler struct {
	tradingSvc domain.TradingService
}

func NewTradeHandler(tradingSvc domain.TradingService) *TradeHandler {
	return &TradeHandler{tradingSvc: tradingSvc}
}

// GetTrades 获取成交记录列表
func (h *TradeHandler) GetTrades(c *fiber.Ctx) error {
	investorID := c.Params("investorID")
	page, _ := strconv.Atoi(c.Query("page", "1"))
	pageSize, _ := strconv.Atoi(c.Query("pageSize", "50"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}

	trades, total, err := h.tradingSvc.GetTrades(context.Background(), investorID, page, pageSize)
	if err != nil {
		return handleError(c, err)
	}

	return c.JSON(fiber.Map{
		"Data": trades,
		"Pagination": fiber.Map{
			"Page":     page,
			"PageSize": pageSize,
			"Total":    total,
		},
	})
}
