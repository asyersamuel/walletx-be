package auth

import "github.com/google/uuid"

type GoogleAuthInput struct {
	IDToken string `json:"id_token"`
}

type UserResponse struct {
	ID       uuid.UUID `json:"id"`
	GoogleID string    `json:"google_id"`
	Email    string    `json:"email"`
	Name     string    `json:"name"`
	Picture  string    `json:"picture"`
}

type GoogleAuthResponse struct {
	User  UserResponse `json:"user"`
	Token string       `json:"token"`
}

func toUserResponse(u *User) UserResponse {
	return UserResponse{
		ID:       u.ID,
		GoogleID: u.GoogleID,
		Email:    u.Email,
		Name:     u.Name,
		Picture:  u.Picture,
	}
}
