package api

import (
	"encoding/json"
	"strconv"	

	"github.com/gofiber/fiber/v2"
	"hhwtrade.com/internal/engine"
	"hhwtrade.com/internal/model"
)

type StrategyHandler struct {
	eng *engine.Engine
}

func NewStrategyHandler(eng *engine.Engine) *StrategyHandler {
	return &StrategyHandler{eng: eng}
}

// CreateStrategy creates a new trading strategy.
// POST /api/strategies
func (h *StrategyHandler) CreateStrategy(c *fiber.Ctx) error {
	var req struct {
		UserID       string             `json:"UserID"`
		InstrumentID string             `json:"InstrumentID"`
		Type         model.StrategyType `json:"Type"`
		Config       json.RawMessage    `json:"Config"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"Error": "Invalid request body"})
	}

	strategy := model.Strategy{
		UserID:       req.UserID,
		InstrumentID: req.InstrumentID,
		Type:         req.Type,
		Status:       model.StrategyStatusActive,
		Config:       req.Config,
	}

	db := h.eng.GetPostgresClient().DB
	if err := db.Create(&strategy).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Failed to create strategy"})
	}

	// TODO: Notify Engine to load this strategy into memory immediately

	return c.Status(fiber.StatusCreated).JSON(strategy)
}

// GetStrategies returns all strategies for a user.
// GET /api/users/:userID/strategies
func (h *StrategyHandler) GetStrategies(c *fiber.Ctx) error {
	userID := c.Params("userID")
	page, _ := strconv.Atoi(c.Query("page", "1"))
	pageSize, _ := strconv.Atoi(c.Query("pageSize", "20"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	offset := (page - 1) * pageSize

	var strategies []model.Strategy
	var total int64

	db := h.eng.GetPostgresClient().DB
	query := db.Model(&model.Strategy{}).Where("user_id = ?", userID)

	if err := query.Count(&total).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Failed to count strategies"})
	}

	if err := query.Order("id DESC").Limit(pageSize).Offset(offset).Find(&strategies).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Failed to fetch strategies"})
	}

	return SendPaginatedResponse(c, strategies, page, pageSize, total)
}

// StopStrategy stops a running strategy.
// POST /api/strategies/:id/stop
func (h *StrategyHandler) StopStrategy(c *fiber.Ctx) error {
	id := c.Params("id")

	db := h.eng.GetPostgresClient().DB
	if err := db.Model(&model.Strategy{}).Where("id = ?", id).Update("status", model.StrategyStatusStopped).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Failed to stop strategy"})
	}

	// TODO: Notify Engine to unload this strategy from memory

	return c.SendStatus(fiber.StatusOK)
}

// StartStrategy starts a stopped strategy.
// POST /api/strategies/:id/start
func (h *StrategyHandler) StartStrategy(c *fiber.Ctx) error {
	id := c.Params("id")

	db := h.eng.GetPostgresClient().DB
	if err := db.Model(&model.Strategy{}).Where("id = ?", id).Update("status", model.StrategyStatusActive).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Failed to start strategy"})
	}

	// TODO: Notify Engine to load this strategy into memory immediately

	return c.SendStatus(fiber.StatusOK)
}

// GetStrategy returns a single strategy by its ID.
// GET /api/strategies/:id
func (h *StrategyHandler) GetStrategy(c *fiber.Ctx) error {
	id := c.Params("id")
	var strategy model.Strategy
	db := h.eng.GetPostgresClient().DB

	if err := db.Where("id = ?", id).First(&strategy).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"Error": "Strategy not found"})
	}

	return c.JSON(strategy)
}

// UpdateStrategy updates a strategy's configuration or other properties.
// PUT /api/strategies/:id
func (h *StrategyHandler) UpdateStrategy(c *fiber.Ctx) error {
	id := c.Params("id")
	db := h.eng.GetPostgresClient().DB

	var strategy model.Strategy
	if err := db.Where("id = ?", id).First(&strategy).Error; err != nil {
		return c.Status(404).JSON(fiber.Map{"Error": "Strategy not found"})
	}

	var req struct {
		Config       json.RawMessage    `json:"Config"`
		InstrumentID string             `json:"InstrumentID"`
		Type         model.StrategyType `json:"Type"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"Error": "Invalid request body"})
	}

	updates := map[string]interface{}{}
	if req.Config != nil {
		updates["Config"] = req.Config
	}
	if req.InstrumentID != "" {
		updates["InstrumentID"] = req.InstrumentID
	}
	if req.Type != "" {
		updates["Type"] = req.Type
	}

	if err := db.Model(&strategy).Updates(updates).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"Error": "Update failed"})
	}

	// TODO: Notify Engine to reload this strategy if it is running

	return c.JSON(strategy)
}

// DeleteStrategy removes a strategy from the DB.
// DELETE /api/strategies/:id
func (h *StrategyHandler) DeleteStrategy(c *fiber.Ctx) error {
	id := c.Params("id")
	db := h.eng.GetPostgresClient().DB

	if err := db.Where("id = ?", id).Delete(&model.Strategy{}).Error; err != nil {
		return c.Status(500).JSON(fiber.Map{"Error": "Delete failed"})
	}

	// TODO: Notify Engine to stop and unload this strategy

	return c.JSON(fiber.Map{"Status": true})
}