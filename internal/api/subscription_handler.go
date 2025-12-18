package api

import (
	"github.com/gofiber/fiber/v2"
	"hhwtrade.com/internal/engine"
	"hhwtrade.com/internal/model"
)

type SubscriptionHandler struct {
	eng *engine.Engine
}

func NewSubscriptionHandler(eng *engine.Engine) *SubscriptionHandler {
	return &SubscriptionHandler{eng: eng}
}

// GetSubscriptions returns the list of symbols subscribed by a user.
// GET /api/users/:userID/subscriptions
func (h *SubscriptionHandler) GetSubscriptions(c *fiber.Ctx) error {
	userID := c.Params("userID")
	var subs []model.UserSubscription

	// Use the Postgres client from the engine
	db := h.eng.GetPostgresClient().DB
	if err := db.Where("user_id = ?", userID).Find(&subs).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch subscriptions"})
	}

	return c.JSON(subs)
}

// AddSubscription adds a symbol to the user's subscription list.
// POST /api/users/:userID/subscriptions
func (h *SubscriptionHandler) AddSubscription(c *fiber.Ctx) error {
	userID := c.Params("userID")
	var req struct {
		Symbol   string `json:"symbol"`
		Exchange string `json:"exchange"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

	sub := model.UserSubscription{
		UserID:   userID,
		Symbol:   req.Symbol,
		Exchange: req.Exchange,
	}

	db := h.eng.GetPostgresClient().DB
	// Use FirstOrCreate to avoid duplicates if unique index is hit, or handle error
	if err := db.Create(&sub).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to add subscription", "details": err.Error()})
	}

	// Trigger WebSocket Subscription
	h.eng.GetWsManager().SubscribeUser(userID, req.Symbol)

	return c.Status(fiber.StatusCreated).JSON(sub)
}

// RemoveSubscription removes a symbol from the user's subscription list.
// DELETE /api/users/:userID/subscriptions/:symbol
func (h *SubscriptionHandler) RemoveSubscription(c *fiber.Ctx) error {
	userID := c.Params("userID")
	symbol := c.Params("symbol")

	db := h.eng.GetPostgresClient().DB
	result := db.Where("user_id = ? AND symbol = ?", userID, symbol).Delete(&model.UserSubscription{})

	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to remove subscription"})
	}

	if result.RowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Subscription not found"})
	}

	// Trigger WebSocket Unsubscription
	h.eng.GetWsManager().UnsubscribeUser(userID, symbol)

	return c.SendStatus(fiber.StatusOK)
}

// SearchInstruments searches for instruments by symbol or name.
// GET /api/instruments?query=rb
func (h *SubscriptionHandler) SearchInstruments(c *fiber.Ctx) error {
	query := c.Query("query")
	if query == "" {
		return c.JSON([]model.Instrument{})
	}

	var instruments []model.Instrument
	db := h.eng.GetPostgresClient().DB

	// Simple case-insensitive approximate search
	// Check database compatibility for ILIKE (Postgres specific)
	if err := db.Where("symbol ILIKE ? OR name ILIKE ?", "%"+query+"%", "%"+query+"%").Limit(20).Find(&instruments).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to search instruments"})
	}

	return c.JSON(instruments)
}
