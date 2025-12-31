package api

import (
	"errors"
	"math"

	"github.com/gofiber/fiber/v2"
	"hhwtrade.com/internal/domain"
)

// Pagination 元数据结构
type Pagination struct {
	Page      int   `json:"Page"`      // 当前页码
	PageSize  int   `json:"PageSize"`  // 每页条数
	Total     int64 `json:"Total"`     // 总记录数
	TotalPage int   `json:"TotalPage"` // 总页数
}

// ListResponse 统一的分页响应结构
type ListResponse struct {
	Data       interface{} `json:"Data"`       // 数据列表
	Pagination Pagination  `json:"Pagination"` // 分页信息
}

// SendPaginatedResponse 发送标准的分页响应
func SendPaginatedResponse(c *fiber.Ctx, data interface{}, page, pageSize int, total int64) error {
	totalPage := 0
	if pageSize > 0 {
		totalPage = int(math.Ceil(float64(total) / float64(pageSize)))
	}

	return c.JSON(ListResponse{
		Data: data,
		Pagination: Pagination{
			Page:      page,
			PageSize:  pageSize,
			Total:     total,
			TotalPage: totalPage,
		},
	})
}

// handleError 统一错误处理
func handleError(c *fiber.Ctx, err error) error {
	// 处理 AppError 类型
	var appErr *domain.AppError
	if errors.As(err, &appErr) {
		return c.Status(appErr.Code).JSON(fiber.Map{"Error": appErr.Message})
	}

	// 处理已知错误类型
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"Error": "Resource not found"})
	case errors.Is(err, domain.ErrInvalidInput):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"Error": "Invalid input"})
	case errors.Is(err, domain.ErrUnauthorized):
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"Error": "Unauthorized"})
	case errors.Is(err, domain.ErrForbidden):
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"Error": "Forbidden"})
	case errors.Is(err, domain.ErrOrderTerminal):
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"Error": "Order already in terminal state"})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"Error": "Internal server error"})
	}
}
