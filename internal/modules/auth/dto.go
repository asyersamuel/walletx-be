package auth

type GoogleAuthInput struct {
	IDToken string `json:"id_token" binding:"required"`
}
