package api

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"hhwtrade.com/internal/domain"
	"hhwtrade.com/internal/model"
)

// StrategyHandler 处理策略相关的 HTTP 请求
type StrategyHandler struct {
	strategySvc domain.StrategyService
}

// NewStrategyHandler 创建策略处理器
func NewStrategyHandler(strategySvc domain.StrategyService) *StrategyHandler {
	return &StrategyHandler{strategySvc: strategySvc}
}

// CreateStrategy 创建策略
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

	strategy := &model.Strategy{
		UserID:       req.UserID,
		InstrumentID: req.InstrumentID,
		Type:         req.Type,
		Status:       model.StrategyStatusActive,
		Config:       req.Config,
	}

	if err := h.strategySvc.CreateStrategy(context.Background(), strategy); err != nil {
		return handleError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(strategy)
}

// GetStrategies 获取用户策略列表
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

	strategies, total, err := h.strategySvc.GetStrategies(context.Background(), userID, page, pageSize)
	if err != nil {
		return handleError(c, err)
	}

	return SendPaginatedResponse(c, strategies, page, pageSize, total)
}

// StopStrategy 停止策略
// POST /api/strategies/:id/stop
func (h *StrategyHandler) StopStrategy(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 32)

	if err := h.strategySvc.StopStrategy(context.Background(), uint(id)); err != nil {
		return handleError(c, err)
	}

	return c.JSON(fiber.Map{"Status": true, "Message": "Strategy stopped"})
}

// StartStrategy 启动策略
// POST /api/strategies/:id/start
func (h *StrategyHandler) StartStrategy(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 32)

	if err := h.strategySvc.StartStrategy(context.Background(), uint(id)); err != nil {
		return handleError(c, err)
	}

	return c.JSON(fiber.Map{"Status": true, "Message": "Strategy started"})
}

// GetStrategy 获取策略详情
// GET /api/strategies/:id
func (h *StrategyHandler) GetStrategy(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 32)

	strategy, err := h.strategySvc.GetStrategy(context.Background(), uint(id))
	if err != nil {
		return handleError(c, err)
	}

	return c.JSON(strategy)
}

// UpdateStrategy 更新策略
// PUT /api/strategies/:id
func (h *StrategyHandler) UpdateStrategy(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 32)

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

	if err := h.strategySvc.UpdateStrategy(context.Background(), uint(id), updates); err != nil {
		return handleError(c, err)
	}

	// 重新获取更新后的策略
	strategy, _ := h.strategySvc.GetStrategy(context.Background(), uint(id))
	return c.JSON(strategy)
}

// DeleteStrategy 删除策略
// DELETE /api/strategies/:id
func (h *StrategyHandler) DeleteStrategy(c *fiber.Ctx) error {
	id, _ := strconv.ParseUint(c.Params("id"), 10, 32)

	if err := h.strategySvc.DeleteStrategy(context.Background(), uint(id)); err != nil {
		return handleError(c, err)
	}

	return c.JSON(fiber.Map{"Status": true})
}
