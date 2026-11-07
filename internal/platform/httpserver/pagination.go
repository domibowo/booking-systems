package httpserver

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

type Pagination struct {
	Page     int
	PageSize int
}

func (p Pagination) Limit() int  { return p.PageSize }
func (p Pagination) Offset() int { return (p.Page - 1) * p.PageSize }

func ParsePagination(c *gin.Context) Pagination {
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}

	pageSize, err := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err != nil || pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	return Pagination{Page: page, PageSize: pageSize}
}
