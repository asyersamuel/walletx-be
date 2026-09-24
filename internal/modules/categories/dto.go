package categories

import "github.com/google/uuid"

// CreateCategoryRequest is the payload for POST /api/v1/categories.
// Both `name` and `type` are required; unknown JSON fields are rejected by
// the handler's strict decoder (DisallowUnknownFields) and all field
// validation (trim, length, enum) lives in the service layer.
type CreateCategoryRequest struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// UpdateCategoryRequest is the payload for PUT /api/v1/categories/:id.
// Both `name` and `type` are required on update; validation lives in the
// service layer.
type UpdateCategoryRequest struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// CategoryResponse is the outbound representation of a single category.
// `user_id` is intentionally omitted from the client-facing response.
type CategoryResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	CreatedAt string    `json:"created_at"`
	UpdatedAt string    `json:"updated_at"`
}

type CreateCategoryResponse struct {
	Category CategoryResponse `json:"category"`
}

type ListCategoriesResponse struct {
	Categories []CategoryResponse `json:"categories"`
}

func toCategoryResponse(c *Category) CategoryResponse {
	return CategoryResponse{
		ID:        c.ID,
		Name:      c.Name,
		Type:      c.Type,
		CreatedAt: c.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt: c.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

func toCategoryResponseList(categories []Category) []CategoryResponse {
	result := make([]CategoryResponse, len(categories))
	for i := range categories {
		result[i] = toCategoryResponse(&categories[i])
	}
	return result
}
