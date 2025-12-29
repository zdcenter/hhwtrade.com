package api

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
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
	UserID       string               `json:"UserID"`
	InstrumentID string               `json:"InstrumentID"`
	Direction    model.OrderDirection `json:"Direction"`
	Offset       model.OrderOffset    `json:"CombOffsetFlag"`
	Price        float64              `json:"LimitPrice"`
	Volume       int                  `json:"VolumeTotalOriginal"`
	StrategyID   *uint                `json:"StrategyID"`
}

// InsertOrder handles ordinary order placement.
// POST /api/trade/order
func (h *TradeHandler) InsertOrder(c *fiber.Ctx) error {
	var req OrderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"Error": "Invalid request body"})
	}

	// 1. Generate unique OrderRef using timestamp (pure in-memory, fastest)
	// Format: Unix timestamp (last 6 digits) + microseconds (6 digits) = 12 chars
	now := time.Now()
	timestampPart := now.Unix() % 1000000
	microPart := now.Nanosecond() / 1000
	orderRef := fmt.Sprintf("%06d%06d", timestampPart, microPart)

	// 2. Send Command to CTP FIRST (minimize latency)
	// Aligned with CThostFtdcInputOrderField
	cmdPayload := map[string]interface{}{
		"InstrumentID":        req.InstrumentID,
		"OrderRef":            orderRef,
		"Direction":           string(req.Direction),
		"CombOffsetFlag":      string(req.Offset),
		"CombHedgeFlag":       "1", // '1' is Speculation (投机)
		"LimitPrice":          req.Price,
		"VolumeTotalOriginal": req.Volume,
		"OrderPriceType":      "2", // '2' is LimitPrice (限价)
		"TimeCondition":       "3", // '3' is GFD (当日有效)
		"VolumeCondition":     "1", // '1' is any volume (任意数量)
		"ContingentCondition": "1", // '1' is immediately (立即触发)
		"ForceCloseReason":    "0", // '0' is not force close (非强平)
	}

	tradeCmd := infra.Command{
		Type:      "INSERT_ORDER",
		Payload:   cmdPayload,
		RequestID: orderRef,
	}

	if err := h.eng.SendCommand(context.Background(), tradeCmd); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Failed to send order to gateway"})
	}

	// 3. Write to DB asynchronously (non-blocking)
	// Even if DB write fails, CTP will send back RTN_ORDER which will create/update the record
	go func() {
		order := model.Order{
			UserID:              req.UserID,
			InstrumentID:        req.InstrumentID,
			Direction:           req.Direction,
			CombOffsetFlag:      req.Offset,
			LimitPrice:          req.Price,
			VolumeTotalOriginal: req.Volume,
			OrderStatus:         model.OrderStatusSent,
			OrderRef:            orderRef,
			StrategyID:          req.StrategyID,
		}

		db := h.eng.GetPostgresClient().DB
		if err := db.Create(&order).Error; err != nil {
			// Log error but don't fail the request
			// The order is already sent to CTP, RTN_ORDER will handle it
			log.Printf("Warning: Failed to write order %s to DB: %v", orderRef, err)
			// TODO: Write to failure queue for retry
		}
	}()

	// 4. Return immediately (ultra-low latency response)
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{
		"Message":   "Order sent",
		"OrderRef":  orderRef,
		"RequestID": orderRef,
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
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Failed to fetch positions"})
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
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Failed to fetch orders"})
	}

	return c.JSON(orders)
}

// SyncPositions triggers a position query to CTP Core.
// POST /api/users/:userID/sync-positions
func (h *TradeHandler) SyncPositions(c *fiber.Ctx) error {
	userID := c.Params("userID")
	symbol := c.Query("symbol")
	if err := h.eng.QueryPositions(userID, symbol); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Failed to trigger position sync"})
	}
	return c.SendStatus(fiber.StatusAccepted)
}

// SyncAccount triggers an account query to CTP Core.
// POST /api/users/:userID/sync-account
func (h *TradeHandler) SyncAccount(c *fiber.Ctx) error {
	userID := c.Params("userID")
	if err := h.eng.QueryAccount(userID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Failed to trigger account sync"})
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
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"Error": "Order not found"})
	}

	// Only cancelable if not already terminal
	if order.OrderStatus == model.OrderStatusAllTraded || order.OrderStatus == model.OrderStatusCanceled || order.OrderStatus == model.OrderStatusNoTradeNotQueueing {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"Error": "Order already in terminal state"})
	}

	cmd := infra.Command{
		Type: "CANCEL_ORDER",
		Payload: map[string]interface{}{
			"InstrumentID": order.InstrumentID,
			"OrderRef":     order.OrderRef,
			"FrontID":      order.FrontID,
			"SessionID":    order.SessionID,
			"ActionFlag":   "0", // '0' is Delete (撤单)
		},
		RequestID: "cancel-" + order.OrderRef,
	}

	if err := h.eng.SendCommand(context.Background(), cmd); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Failed to send cancel command"})
	}

	return c.JSON(fiber.Map{"Message": "Cancel request sent"})
}
