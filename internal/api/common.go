package api

import (
	"math"

	"github.com/gofiber/fiber/v2"
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
