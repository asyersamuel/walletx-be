package handlers

import (
	"context"
	"net/http"

	"walletx-be/configs"
	"walletx-be/pkg/services"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type AuthHandler struct {
	authService *services.AuthService
	oauthConfig *oauth2.Config 
}

func NewAuthHandler(authService *services.AuthService, cfg *configs.Config) *AuthHandler {
	conf := &oauth2.Config{
		ClientID: cfg.OAuth.ClientID,
		ClientSecret: cfg.OAuth.ClientSecret,
		RedirectURL: cfg.OAuth.RedirectURL,
		Scopes: []string{"https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/userinfo.profile"},
		Endpoint: google.Endpoint,
	}

	return &AuthHandler{authService: authService, oauthConfig: conf}
}

// HandleGoogleAuth adalah endpoint POST /api/v1/auth/google - PRODUCTION MOBILE APP
func (h *AuthHandler) HandleGoogleAuth(c *gin.Context) {
    var input services.GoogleAuthInput

    if err := c.ShouldBindJSON(&input); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "Data tidak valid", "detail": err.Error()})
        return
    }

    h.processAuthLogic(c, input)
}

// ENDPOINT TESTING - DEVELOPMENT IN BROWSER
func (h *AuthHandler) GoogleLoginTest(c *gin.Context){
	state := "random-state-string"
	url := h.oauthConfig.AuthCodeURL(state)
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func (h *AuthHandler) GoogleCallbackTest(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Code tidak ditemukan dari Google"})
		return
	}

	token, err := h.oauthConfig.Exchange(context.Background(), code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal menukar kode dengan token", "detail": err.Error()})
		return
	}

	idToken, ok := token.Extra("id_token").(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Tidak mendapatkan id_token dari Google"})
		return
	}

	input := services.GoogleAuthInput{
		IDToken: idToken,
	}

	h.processAuthLogic(c, input)
}

// HELPER
func (h *AuthHandler) processAuthLogic(c *gin.Context, input services.GoogleAuthInput) {
	user, isNewUser, err := h.authService.ProcessGoogleAuth(input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal memproses autentikasi", "detail": err.Error()})
		return
	}

	// Buat token internal JWT untuk aplikasimu
	internalToken, err := h.authService.GenerateJWT(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Gagal membuat token internal"})
		return
	}

	message := "Login berhasil"
	statusCode := http.StatusOK
	if isNewUser {
		message = "Registrasi berhasil"
		statusCode = http.StatusCreated
	}

	c.JSON(statusCode, gin.H{
		"status":  "success",
		"message": message,
		"data": gin.H{
			"user":  user,
			"token": internalToken,
		},
	})
}
