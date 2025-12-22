package api

import (
	"log"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"hhwtrade.com/internal/engine"
	"hhwtrade.com/internal/model"
)

type SubscriptionHandler struct {
	eng *engine.Engine
}

func NewSubscriptionHandler(eng *engine.Engine) *SubscriptionHandler {
	return &SubscriptionHandler{eng: eng}
}

// GetSubscriptions returns the list of symbols subscribed by a user with pagination.
// GET /api/users/:userID/subscriptions?page=1&pageSize=10
func (h *SubscriptionHandler) GetSubscriptions(c *fiber.Ctx) error {
	userID := c.Params("userID")

	// Pagination parameters
	page, _ := strconv.Atoi(c.Query("page", "1"))
	pageSize, _ := strconv.Atoi(c.Query("pageSize", "10"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	offset := (page - 1) * pageSize

	var subs []model.UserSubscription
	var total int64

	db := h.eng.GetPostgresClient().DB

	// Count total records
	if err := db.Model(&model.UserSubscription{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to count subscriptions"})
	}

	// Fetch paginated data
	if err := db.Where("user_id = ?", userID).Order("sorter ASC").Limit(pageSize).Offset(offset).Find(&subs).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch subscriptions"})
	}

	return c.JSON(fiber.Map{
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
		"data":     subs,
	})
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

	// Trigger Engine CTP Subscription
	if err := h.eng.SubscribeSymbol(req.Symbol); err != nil {
		log.Printf("API: Failed to trigger CTP subscription for %s: %v", req.Symbol, err)
	}

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

	// Trigger Engine CTP Unsubscription
	if err := h.eng.UnsubscribeSymbol(symbol); err != nil {
		log.Printf("API: Failed to trigger CTP unsubscription for %s: %v", symbol, err)
	}

	// --- 修改这里 ---
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"status":  true,
		"message": "Unsubscribed successfully",
		"symbol":  symbol,
	})
}


// SearchInstruments searches for instruments by symbol, product ID, or name.
// GET /api/instruments?query=rb
func (h *SubscriptionHandler) SearchInstruments(c *fiber.Ctx) error {
	query := c.Query("q")
	if query == "" {
		return c.JSON([]model.FuturesContract{})
	}

	var instruments []model.FuturesContract
	db := h.eng.GetPostgresClient().DB

	// Priority Search:
	// 1. Prefix match on Symbol (e.g. rb%)
	// 2. Exact match on ProductID (e.g. rb)
	// 3. Fuzzy match on Name
	searchTerm := query + "%"
	if err := db.Model(&model.FuturesContract{}).Where("symbol ILIKE ? OR product_id ILIKE ? OR name ILIKE ?", searchTerm, query, "%"+query+"%").
		Order("symbol ASC").
		Limit(50).
		Find(&instruments).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to search instruments"})
	}

	return c.JSON(instruments)
}

// SyncInstruments triggers the background process to fetch and save all instruments from CTP.
// POST /api/instruments/sync
func (h *SubscriptionHandler) SyncInstruments(c *fiber.Ctx) error {
	if err := h.eng.SyncInstruments(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to trigger instrument sync"})
	}
	return c.JSON(fiber.Map{"message": "Instrument synchronization triggered"})
}


func (h *SubscriptionHandler) ReorderSubscriptions(c *fiber.Ctx) error {
	userID := c.Params("userID")
	var req struct {
		Symbols []string `json:"symbols"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

  db := h.eng.GetPostgresClient().DB
    // 开启事务批量更新排序号
    err := db.Transaction(func(tx *gorm.DB) error {
        for i, symbol := range req.Symbols {
            if err := tx.Model(&model.UserSubscription{}).
                Where("user_id = ? AND symbol = ?", userID, symbol).
                Update("sorter", i).Error; err != nil {
                return err
            }
        }
        return nil
    })
    
    if err != nil {
        return c.Status(500).JSON(fiber.Map{"error": "Failed to reorder"})
    }
    return c.JSON(fiber.Map{"status": true})
}	