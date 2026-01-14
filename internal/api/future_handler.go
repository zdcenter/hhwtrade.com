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
// @Summary 获取期货合约列表
// @Description 分页获取期货列表，支持按InstrumentID(前缀)和ExchangeID过滤
// @Tags Market
// @Accept json
// @Produce json
// @Param page query int false "页码" default(1)
// @Param pageSize query int false "每页数量" default(50)
// @Param InstrumentID query string false "按合约代码前缀筛选"
// @Param ExchangeID query string false "按交易所代码筛选"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /futures [get]
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
// @Summary 获取单个合约详情
// @Description 根据合约ID获取单个期货合约的详细信息
// @Tags Market
// @Accept json
// @Produce json
// @Param id path string true "合约ID (InstrumentID)"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Security BearerAuth
// @Router /futures/{id} [get]
func (h *FutureHandler) GetFuture(c *fiber.Ctx) error {
	id := c.Params("id")
	var instrument model.Future

	if err := h.db.Where("instrument_id = ?", id).First(&instrument).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"Error": "Instrument not found"})
	}

	return c.JSON(fiber.Map{"Status": true, "Data": instrument})
}

// UpdateFuture 更新合约
// @Summary 更新合约信息
// @Description 更新指定ID的期货合约信息
// @Tags Market
// @Accept json
// @Produce json
// @Param id path string true "合约ID"
// @Param body body model.Future true "合约信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /futures/{id} [put]
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
// @Summary 删除合约
// @Description 删除指定ID的期货合约
// @Tags Market
// @Accept json
// @Produce json
// @Param id path string true "合约ID"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /futures/{id} [delete]
func (h *FutureHandler) DeleteFuture(c *fiber.Ctx) error {
	id := c.Params("id")

	if err := h.db.Where("instrument_id = ?", id).Delete(&model.Future{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"Error": "Delete failed"})
	}

	return c.JSON(fiber.Map{"Status": true})
}

// SearchInstruments 搜索合约
// @Summary 搜索合约
// @Description 根据关键字模糊搜索合约（匹配ID、产品ID或名称）
// @Tags Market
// @Accept json
// @Produce json
// @Param q query string true "搜索关键字"
// @Success 200 {array} model.Future
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /futures/search [get]
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
// @Summary 同步合约数据
// @Description 触发后台任务，从 CTP 或其他源同步最新的合约列表
// @Tags Market
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /futures/sync [post]
func (h *FutureHandler) SyncInstruments(c *fiber.Ctx) error {
	if err := h.marketSvc.SyncInstruments(c.Context()); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Failed to trigger instrument sync"})
	}
	return c.JSON(fiber.Map{"Status": true, "Message": "Instrument synchronization triggered"})
}

// CleanupExpired 清理过期合约
// @Summary 清理过期合约
// @Description 删除数据库中所有过期日期早于今天的合约记录
// @Tags Market
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /futures/cleanup [post]
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

// UpdateFutureRates 更新合约保证金和手续费率
// @Summary 更新合约保证金和手续费率
// @Description 触发从 CTP 查询指定合约的最新保证金和手续费率，并更新到数据库
// @Tags Market
// @Accept json
// @Produce json
// @Param id path string true "合约ID (InstrumentID)"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Security BearerAuth
// @Router /futures/{id}/rates [post]
func (h *FutureHandler) UpdateFutureRates(c *fiber.Ctx) error {
	instrumentID := c.Params("id")

	// 触发费率更新 (内部包含 1.2s 延迟以避开 CTP 流控)
	if err := h.marketSvc.RequestRateUpdate(c.Context(), instrumentID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Failed to trigger rate update: " + err.Error()})
	}

	return c.JSON(fiber.Map{"Status": true, "Message": "Rate update triggered for " + instrumentID})
}
