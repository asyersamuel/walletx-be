package category

type CreateInput struct {
	Name string `json:"name" binding:"required"`
	Icon string `json:"icon"`
}

type UpdateInput struct {
	Name string `json:"name" binding:"required"`
	Icon string `json:"icon"`
}
