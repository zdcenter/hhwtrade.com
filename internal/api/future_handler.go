package api

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
	"hhwtrade.com/internal/domain"
	"hhwtrade.com/internal/model"
)

// FutureHandler 处理期货合约相关的 HTTP 请求
type FutureHandler struct {
	db        *gorm.DB
	marketSvc domain.MarketService
}

// NewFutureHandler 创建期货合约处理器
func NewFutureHandler(db *gorm.DB, marketSvc domain.MarketService) *FutureHandler {
	return &FutureHandler{
		db:        db,
		marketSvc: marketSvc,
	}
}

// GetFutures 获取期货合约列表
// GET /api/futures
func (h *FutureHandler) GetFutures(c *fiber.Ctx) error {
	page, _ := strconv.Atoi(c.Query("page", "1"))
	pageSize, _ := strconv.Atoi(c.Query("pageSize", "50"))
	instrumentID := c.Query("InstrumentID")
	exchangeID := c.Query("ExchangeID")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 500 {
		pageSize = 50
	}

	offset := (page - 1) * pageSize

	var instruments []model.Future
	var total int64

	query := h.db.Model(&model.Future{})

	if instrumentID != "" {
		query = query.Where("instrument_id ILIKE ?", instrumentID+"%")
	}
	if exchangeID != "" {
		query = query.Where("exchange_id = ?", exchangeID)
	}

	if err := query.Count(&total).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"Error": "Database error"})
	}

	if err := query.Order("instrument_id ASC").Limit(pageSize).Offset(offset).Find(&instruments).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"Error": "Database error"})
	}

	return SendPaginatedResponse(c, instruments, page, pageSize, total)
}

// GetFuture 获取单个合约
// GET /api/futures/:id
func (h *FutureHandler) GetFuture(c *fiber.Ctx) error {
	id := c.Params("id")
	var instrument model.Future

	if err := h.db.Where("instrument_id = ?", id).First(&instrument).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"Error": "Instrument not found"})
	}

	return c.JSON(fiber.Map{"Status": true, "Data": instrument})
}

// UpdateFuture 更新合约
// PUT /api/futures/:id
func (h *FutureHandler) UpdateFuture(c *fiber.Ctx) error {
	id := c.Params("id")

	var instrument model.Future
	if err := h.db.Where("instrument_id = ?", id).First(&instrument).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"Error": "Instrument not found"})
	}

	if err := c.BodyParser(&instrument); err != nil {
		return c.Status(400).JSON(fiber.Map{"Error": "Invalid body"})
	}

	if err := h.db.Save(&instrument).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"Error": "Update failed"})
	}

	return c.JSON(fiber.Map{"Status": true, "Data": instrument})
}

// DeleteFuture 删除合约
// DELETE /api/futures/:id
func (h *FutureHandler) DeleteFuture(c *fiber.Ctx) error {
	id := c.Params("id")

	if err := h.db.Where("instrument_id = ?", id).Delete(&model.Future{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"Error": "Delete failed"})
	}

	return c.JSON(fiber.Map{"Status": true})
}

// SearchInstruments 搜索合约
// GET /api/futures/search?q=rb
func (h *FutureHandler) SearchInstruments(c *fiber.Ctx) error {
	query := c.Query("q")
	if query == "" {
		return c.JSON([]model.Future{})
	}

	var instruments []model.Future
	searchTerm := query + "%"

	if err := h.db.Model(&model.Future{}).
		Where("instrument_id ILIKE ? OR product_id ILIKE ? OR instrument_name ILIKE ?", searchTerm, query, "%"+query+"%").
		Order("instrument_id ASC").
		Limit(50).
		Find(&instruments).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Failed to search instruments"})
	}

	return c.JSON(instruments)
}

// SyncInstruments 同步合约
// POST /api/futures/sync
func (h *FutureHandler) SyncInstruments(c *fiber.Ctx) error {
	if err := h.marketSvc.SyncInstruments(c.Context()); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Failed to trigger instrument sync"})
	}
	return c.JSON(fiber.Map{"Status": true, "Message": "Instrument synchronization triggered"})
}

// CleanupExpired 清理过期合约
// POST /api/futures/cleanup
func (h *FutureHandler) CleanupExpired(c *fiber.Ctx) error {
	now := time.Now().Format("20060102")

	result := h.db.Where("expire_date < ? AND expire_date != ''", now).Delete(&model.Future{})
	if result.Error != nil {
		return c.Status(500).JSON(fiber.Map{"Error": "Cleanup failed: " + result.Error.Error()})
	}

	return c.JSON(fiber.Map{
		"Status":  true,
		"Message": strconv.FormatInt(result.RowsAffected, 10) + " expired instruments removed",
	})
}
