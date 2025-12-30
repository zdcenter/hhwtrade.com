package api

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"hhwtrade.com/internal/engine"
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

	// 2. Prepare Order Model
	order := model.Order{
		UserID:              req.UserID,
		InstrumentID:        req.InstrumentID,
		OrderRef:            orderRef,
		Direction:           req.Direction,
		CombOffsetFlag:      req.Offset,
		LimitPrice:          req.Price,
		VolumeTotalOriginal: req.Volume,
		OrderStatus:         model.OrderStatusSent,
		StrategyID:          req.StrategyID,
		// ExchangeID, InvestorID will be filled/handled by CTP Client or defaults
	}

	// 3. Send Command to CTP via Client
	if err := h.eng.GetCtpClient().InsertOrder(context.Background(), &order); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Failed to send order to gateway"})
	}

	// 4. Write to DB asynchronously (non-blocking)
	go func() {
		db := h.eng.GetPostgresClient().DB
		if err := db.Create(&order).Error; err != nil {
			log.Printf("Warning: Failed to write order %s to DB: %v", orderRef, err)
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
	page, _ := strconv.Atoi(c.Query("page", "1"))
	pageSize, _ := strconv.Atoi(c.Query("pageSize", "50"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}
	offset := (page - 1) * pageSize

	var orders []model.Order
	var total int64

	db := h.eng.GetPostgresClient().DB
	query := db.Model(&model.Order{}).Where("user_id = ?", userID)

	if err := query.Count(&total).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Failed to count orders"})
	}

	// Order by time desc
	if err := query.Order("created_at desc").Limit(pageSize).Offset(offset).Find(&orders).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Failed to fetch orders"})
	}

	return SendPaginatedResponse(c, orders, page, pageSize, total)
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

	if err := h.eng.GetCtpClient().CancelOrder(context.Background(), &order); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Failed to send cancel command"})
	}

	return c.JSON(fiber.Map{"Message": "Cancel request sent"})
}
