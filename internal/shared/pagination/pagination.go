package pagination

import "math"

// Params містить параметри для пагінації та сортування з HTTP запитів
type Params struct {
	Page   int    `form:"page,default=1" binding:"min=1"`
	Limit  int    `form:"limit,default=20" binding:"min=1,max=100"`
	SortBy string `form:"sort_by,default=created_at"`
	Order  string `form:"order,default=desc" binding:"oneof=asc desc"`
}

// GetOffset розраховує зміщення для SQL запиту
func (p *Params) GetOffset() int {
	if p.Page <= 1 {
		return 0
	}
	return (p.Page - 1) * p.Limit
}

// Metadata містить інформацію про сторінки, яка повертається фронтенду
type Metadata struct {
	CurrentPage int   `json:"current_page"`
	PageSize    int   `json:"page_size"`
	TotalItems  int64 `json:"total_items"`
	TotalPages  int   `json:"total_pages"`
	HasNextPage bool  `json:"has_next_page"`
	HasPrevPage bool  `json:"has_prev_page"`
}

// CalculateMetadata формує метадані на основі загальної кількості записів
func CalculateMetadata(totalItems int64, page, limit int) Metadata {
	totalPages := int(math.Ceil(float64(totalItems) / float64(limit)))

	if totalPages == 0 {
		totalPages = 1
	}

	return Metadata{
		CurrentPage: page,
		PageSize:    limit,
		TotalItems:  totalItems,
		TotalPages:  totalPages,
		HasNextPage: page < totalPages,
		HasPrevPage: page > 1,
	}
}

// PagedResponse - загальна структура для пагінованої відповіді
type PagedResponse struct {
	Metadata Metadata    `json:"metadata"`
	Data     interface{} `json:"data"`
}
