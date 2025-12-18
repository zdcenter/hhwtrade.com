package api

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"hhwtrade.com/internal/engine"
	"hhwtrade.com/internal/infra"
	"hhwtrade.com/internal/model"
)

type TradeHandler struct {
	eng *engine.Engine
}

func NewTradeHandler(eng *engine.Engine) *TradeHandler {
	return &TradeHandler{eng: eng}
}

// OrderRequest payload
type OrderRequest struct {
	UserID     string               `json:"user_id"`
	Symbol     string               `json:"symbol"`
	Direction  model.OrderDirection `json:"direction"` // buy, sell
	Offset     model.OrderOffset    `json:"offset"`    // open, close
	Price      float64              `json:"price"`     // Limit price
	Volume     int                  `json:"volume"`
	StrategyID *uint                `json:"strategy_id"` // Optional: for strategy orders
}

// InsertOrder handles ordinary order placement.
// POST /api/trade/order
func (h *TradeHandler) InsertOrder(c *fiber.Ctx) error {
	var req OrderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	// 1. Generate unique RequestID
	reqID := uuid.New().String()

	// 2. [Optional] Validation (Check balance, position via local cache)
	// if failure -> return error immediately

	// 3. Create Order Record in Postgres (Status: Pending)
	order := model.Order{
		UserID:     req.UserID,
		Symbol:     req.Symbol,
		Direction:  req.Direction,
		Offset:     req.Offset,
		Price:      req.Price,
		Volume:     req.Volume,
		Status:     model.OrderStatusPending, // 初始状态：待报送
		RequestID:  reqID,
		StrategyID: req.StrategyID,
	}

	db := h.eng.GetPostgresClient().DB
	if err := db.Create(&order).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to create order record"})
	}

	// 4. Send Command to CTP Core via Redis Queue
	// We need to match the payload format expected by CTP Core
	// Assuming CTP Core expects keys like "instrument", "limit_price", "direction_char" etc.
	// For now we pass a JSON map.
	cmdPayload := map[string]interface{}{
		"request_id":  reqID,
		"user_id":     req.UserID,
		"symbol":      req.Symbol,
		"price":       req.Price,
		"volume":      req.Volume,
		"direction":   req.Direction, // maybe need mapping to '0'/'1'
		"offset":      req.Offset,    // maybe need mapping to '0'/'1'
		"strategy_id": req.StrategyID,
	}

	tradeCmd := infra.TradeCommand{
		Type:      "INSERT_ORDER",
		Payload:   cmdPayload,
		RequestID: reqID,
	}

	if err := h.eng.SendCommand(context.Background(), tradeCmd); err != nil {
		// If Redis push failed, mark order as Error
		db.Model(&order).Update("status", model.OrderStatusRejected)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to send order to gateway"})
	}

	// 5. Update Order Status to Sent (Optimistic)
	db.Model(&order).Update("status", model.OrderStatusSent)

	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"message":    "Order accepted",
		"order_id":   order.ID,
		"request_id": reqID,
	})
}

// GetPositions returns the user's current positions.
// GET /api/users/:userID/positions
func (h *TradeHandler) GetPositions(c *fiber.Ctx) error {
	userID := c.Params("userID")
	var positions []model.Position

	// Query from Local DB (Synchronized from CTP)
	db := h.eng.GetPostgresClient().DB
	if err := db.Where("user_id = ?", userID).Find(&positions).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch positions"})
	}

	return c.JSON(positions)
}

// GetOrders returns the user's order history.
// GET /api/users/:userID/orders
func (h *TradeHandler) GetOrders(c *fiber.Ctx) error {
	userID := c.Params("userID")

	// Pagination params could be added here

	var orders []model.Order
	db := h.eng.GetPostgresClient().DB

	// Order by time desc
	if err := db.Where("user_id = ?", userID).Order("created_at desc").Limit(50).Find(&orders).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch orders"})
	}

	return c.JSON(orders)
}

// SyncPositions triggers a position query to CTP Core.
// POST /api/users/:userID/sync-positions
func (h *TradeHandler) SyncPositions(c *fiber.Ctx) error {
	userID := c.Params("userID")
	if err := h.eng.QueryPositions(userID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to trigger position sync"})
	}
	return c.SendStatus(fiber.StatusAccepted)
}
