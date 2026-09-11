package pagination

// Meta represents pagination metadata
type Meta struct {
	Page       int `json:"page"`
	Limit      int `json:"limit"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

// CalculateMeta calculates pagination metadata from total count, limit, and offset
func CalculateMeta(total int, limit int, offset int) Meta {
	if limit <= 0 {
		limit = 20
	}

	page := (offset / limit) + 1
	if offset == 0 {
		page = 1
	}

	totalPages := (total + limit - 1) / limit
	if totalPages == 0 {
		totalPages = 1
	}

	return Meta{
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
	}
}
