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
	// Map internal constants to CTP chars
	directionChar := "0" // Buy
	if req.Direction == model.DirectionSell {
		directionChar = "1"
	}

	offsetChar := "0" // Open
	if req.Offset == model.OffsetClose {
		offsetChar = "1"
	} else if req.Offset == model.OffsetCloseToday {
		offsetChar = "3"
	}

	cmdPayload := map[string]interface{}{
		"symbol":      req.Symbol,
		"price":       req.Price,
		"volume":      req.Volume,
		"direction":   directionChar,
		"offset":      offsetChar,
		"order_ref":   reqID, // Use reqID as order_ref for simple tracking
		"strategy_id": req.StrategyID,
	}

	tradeCmd := infra.Command{
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
	symbol := c.Query("symbol")
	if err := h.eng.QueryPositions(userID, symbol); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to trigger position sync"})
	}
	return c.SendStatus(fiber.StatusAccepted)
}

// SyncAccount triggers an account query to CTP Core.
// POST /api/users/:userID/sync-account
func (h *TradeHandler) SyncAccount(c *fiber.Ctx) error {
	userID := c.Params("userID")
	if err := h.eng.QueryAccount(userID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to trigger account sync"})
	}
	return c.SendStatus(fiber.StatusAccepted)
}

// CancelOrder handles order cancellation.
// POST /api/trade/order/:id/cancel
func (h *TradeHandler) CancelOrder(c *fiber.Ctx) error {
	orderID := c.Params("id")
	db := h.eng.GetPostgresClient().DB

	var order model.Order
	if err := db.First(&order, orderID).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Order not found"})
	}

	// Only cancelable if not already terminal
	if order.Status == model.OrderStatusFilled || order.Status == model.OrderStatusCanceled || order.Status == model.OrderStatusRejected {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Order already in terminal state"})
	}

	cmd := infra.Command{
		Type: "CANCEL_ORDER",
		Payload: map[string]interface{}{
			"symbol":    order.Symbol,
			"order_ref": order.RequestID, // We used RequestID as OrderRef
			// "front_id": order.FrontID, // These should be saved when RTN_ORDER comes back
			// "session_id": order.SessionID,
		},
		RequestID: "cancel-" + order.RequestID,
	}

	if err := h.eng.SendCommand(context.Background(), cmd); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to send cancel command"})
	}

	return c.JSON(fiber.Map{"message": "Cancel request sent"})
}
