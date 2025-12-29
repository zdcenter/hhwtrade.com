package api

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"hhwtrade.com/internal/engine"
	"hhwtrade.com/internal/model"
)

type FutureHandler struct {
	eng *engine.Engine
}

func NewFutureHandler(eng *engine.Engine) *FutureHandler {
	return &FutureHandler{eng: eng}
}

// GetFutures returns a paginated list of available future contracts.
// GET /api/futures
func (h *FutureHandler) GetFutures(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("Page", "1"))
	limit, _ := strconv.Atoi(c.Query("Limit", "50"))
	instrumentID := c.Query("InstrumentID")
	exchangeID := c.Query("ExchangeID")

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 500 {
		limit = 50
	}

	offset := (page - 1) * limit

	var instruments []model.Future
	var total int64

	db := h.eng.GetPostgresClient().DB
	query := db.Model(&model.Future{})

	if instrumentID != "" {
		query = query.Where("instrument_id ILIKE ?", instrumentID+"%")
	}
	if exchangeID != "" {
		query = query.Where("exchange_id = ?", exchangeID)
	}

	if err := query.Count(&total).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"Error": "Database error"})
	}

	if err := query.Order("instrument_id ASC").Limit(limit).Offset(offset).Find(&instruments).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"Error": "Database error"})
	}

	return c.JSON(fiber.Map{
		"Status": true,
		"Data": fiber.Map{
			"Items": instruments,
			"Pagination": fiber.Map{
				"Total":       total,
				"Limit":       limit,
				"CurrentPage": page,
			},
		},
	})
}

// GetFuture returns a single instrument by its ID.
// GET /api/futures/:id
func (h *FutureHandler) GetFuture(c *fiber.Ctx) error {
	id := c.Params("id")
	var instrument model.Future
	db := h.eng.GetPostgresClient().DB

	if err := db.Where("instrument_id = ?", id).First(&instrument).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"Error": "Instrument not found"})
	}

	return c.JSON(fiber.Map{"Status": true, "Data": instrument})
}

// UpdateFuture updates an instrument's properties (like IsActive).
// PUT /api/futures/:id
func (h *FutureHandler) UpdateFuture(c *fiber.Ctx) error {
	id := c.Params("id")
	db := h.eng.GetPostgresClient().DB

	var instrument model.Future
	if err := db.Where("instrument_id = ?", id).First(&instrument).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"Error": "Instrument not found"})
	}

	if err := c.BodyParser(&instrument); err != nil {
		return c.Status(400).JSON(fiber.Map{"Error": "Invalid body"})
	}

	if err := db.Save(&instrument).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"Error": "Update failed"})
	}

	return c.JSON(fiber.Map{"Status": true, "Data": instrument})
}

// DeleteFuture removes an instrument from the DB.
// DELETE /api/futures/:id
func (h *FutureHandler) DeleteFuture(c *fiber.Ctx) error {
	id := c.Params("id")
	db := h.eng.GetPostgresClient().DB

	if err := db.Where("instrument_id = ?", id).Delete(&model.Future{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"Error": "Delete failed"})
	}

	return c.JSON(fiber.Map{"Status": true})
}

// SearchInstruments searches for instruments by symbol, product ID, or name.
// GET /api/futures/search?q=rb
func (h *FutureHandler) SearchInstruments(c *fiber.Ctx) error {
	query := c.Query("q")
	if query == "" {
		return c.JSON([]model.Future{})
	}

	var instruments []model.Future
	db := h.eng.GetPostgresClient().DB

	searchTerm := query + "%"
	if err := db.Model(&model.Future{}).Where("instrument_id ILIKE ? OR product_id ILIKE ? OR instrument_name ILIKE ?", searchTerm, query, "%"+query+"%").
		Order("instrument_id ASC").
		Limit(50).
		Find(&instruments).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Failed to search instruments"})
	}

	return c.JSON(instruments)
}

// SyncInstruments triggers the background process to fetch and save all instruments from CTP.
// POST /api/futures/sync
func (h *FutureHandler) SyncInstruments(c *fiber.Ctx) error {
	if err := h.eng.SyncInstruments(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Failed to trigger instrument sync"})
	}
	return c.JSON(fiber.Map{"Status": true, "Message": "Instrument synchronization triggered"})
}

// CleanupExpired removes expired instruments from the DB.
// POST /api/futures/cleanup
func (h *FutureHandler) CleanupExpired(c *fiber.Ctx) error {
	db := h.eng.GetPostgresClient().DB
	now := time.Now().Format("20060102")

	result := db.Where("expire_date < ? AND expire_date != ''", now).Delete(&model.Future{})
	if result.Error != nil {
		return c.Status(500).JSON(fiber.Map{"Error": "Cleanup failed: " + result.Error.Error()})
	}

	return c.JSON(fiber.Map{
		"Status": true,
		"Message": strconv.FormatInt(result.RowsAffected, 10) + " expired instruments removed",
	})
}
