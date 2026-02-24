package handlers

import (
	"net/http"
	"walletx-be/internal/services"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	authService *services.AuthService
}

func NewAuthHandler(authService *services.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

// HandleGoogleAuth adalah endpoint POST /api/v1/auth/google
func (h *AuthHandler) HandleGoogleAuth(c *gin.Context) {
	var input services.GoogleAuthInput

	// 1. Tangkap JSON Request
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Data tidak valid", "detail": err.Error()})
		return
	}

	// 2. Serahkan ke Service
	user, isNewUser, err := h.authService.ProcessGoogleAuth(input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal memproses autentikasi"})
		return
	}

	// 3. Berikan Response
	message := "Login berhasil"
	if isNewUser {
		message = "Registrasi berhasil"
		c.JSON(http.StatusCreated, gin.H{
			"status":  "success",
			"message": message,
			"data":    user,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": message,
		"data":    user,
	})
}