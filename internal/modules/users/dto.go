package users

// UpdateUserRequest is the mutable profile payload for PUT /users/me.
// Identity fields (id, google_id, email) and timestamps are intentionally
// excluded so clients cannot modify them.
type UpdateUserRequest struct {
	Name    string  `json:"name" binding:"required,max=100"`
	Picture *string `json:"picture" binding:"omitempty,max=2048"`
}
