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

	var subs []model.Subscription
	var total int64

	db := h.eng.GetPostgresClient().DB

	// Count total records
	if err := db.Model(&model.Subscription{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Failed to count subscriptions"})
	}

	// Fetch paginated data
	if err := db.Where("user_id = ?", userID).Order("sorter ASC").Limit(pageSize).Offset(offset).Find(&subs).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Failed to fetch subscriptions"})
	}

	return SendPaginatedResponse(c, subs, page, pageSize, total)
}

// AddSubscription adds an instrument_id to the user's subscription list.
// POST /api/users/:userID/subscriptions
func (h *SubscriptionHandler) AddSubscription(c *fiber.Ctx) error {
	userID := c.Params("userID")
	var req struct {
		InstrumentID string `json:"InstrumentID"`
		ExchangeID   string `json:"ExchangeID"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"Error": "Invalid request body"})
	}

	sub := model.Subscription{
		UserID:       userID,
		InstrumentID: req.InstrumentID,
		ExchangeID:   req.ExchangeID,
	}

	db := h.eng.GetPostgresClient().DB
	// Use FirstOrCreate to avoid duplicates if unique index is hit, or handle error
	if err := db.Create(&sub).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Failed to add subscription", "Details": err.Error()})
	}

	// Trigger WebSocket Subscription
	h.eng.GetWsManager().SubscribeUser(userID, req.InstrumentID)

	// Trigger Engine CTP Subscription
	if err := h.eng.SubscribeSymbol(req.InstrumentID); err != nil {
		log.Printf("API: Failed to trigger CTP subscription for %s: %v", req.InstrumentID, err)
	}

	return c.Status(fiber.StatusCreated).JSON(sub)
}

// RemoveSubscription removes a symbol from the user's subscription list.
// DELETE /api/users/:userID/subscriptions/:symbol
func (h *SubscriptionHandler) RemoveSubscription(c *fiber.Ctx) error {
	userID := c.Params("userID")
	instrumentID := c.Params("symbol") // 保持 URL param 名为 symbol 也可以，只要逻辑对

	db := h.eng.GetPostgresClient().DB
	result := db.Where("user_id = ? AND instrument_id = ?", userID, instrumentID).Delete(&model.Subscription{})

	if result.Error != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Failed to remove subscription"})
	}
	if result.RowsAffected == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"Error": "Subscription not found"})
	}

	// Trigger WebSocket Unsubscription
	h.eng.GetWsManager().UnsubscribeUser(userID, instrumentID)

	// Trigger Engine CTP Unsubscription
	if err := h.eng.UnsubscribeSymbol(instrumentID); err != nil {
		log.Printf("API: Failed to trigger CTP unsubscription for %s: %v", instrumentID, err)
	}

	// --- 修改这里 ---
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"Status":       true,
		"Message":      "Unsubscribed successfully",
		"InstrumentID": instrumentID,
	})
}





func (h *SubscriptionHandler) ReorderSubscriptions(c *fiber.Ctx) error {
	userID := c.Params("userID")
	var req struct {
		InstrumentIDs []string `json:"InstrumentIDs"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
	}

  db := h.eng.GetPostgresClient().DB
    // 开启事务批量更新排序号
    err := db.Transaction(func(tx *gorm.DB) error {
        for i, symbol := range req.InstrumentIDs {
            if err := tx.Model(&model.Subscription{}).
                Where("user_id = ? AND instrument_id = ?", userID, symbol).
                Update("sorter", i).Error; err != nil {
                return err
            }
        }
        return nil
    })
    
    if err != nil {
        return c.Status(500).JSON(fiber.Map{"Error": "Failed to reorder"})
    }
    return c.JSON(fiber.Map{"Status": true})
}