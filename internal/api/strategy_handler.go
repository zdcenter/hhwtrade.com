package api

import (
	"encoding/json"

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
	var strategies []model.Strategy

	db := h.eng.GetPostgresClient().DB
	if err := db.Where("user_id = ?", userID).Find(&strategies).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Failed to fetch strategies"})
	}

	return c.JSON(strategies)
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