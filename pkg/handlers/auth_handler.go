package handlers

import (
	"context"
	"net/http"
	"strings"

	"walletx-be/configs"
	"walletx-be/pkg/services"
	"walletx-be/pkg/utils"

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
        utils.FailResponseWithDetails(c, "Data tidak valid", err.Error())
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
		utils.FailResponse(c, "Code tidak ditemukan dari Google")
		return
	}

	token, err := h.oauthConfig.Exchange(context.Background(), code)
	if err != nil {
		utils.ErrorResponseWithDetails(c, "Gagal menukar kode dengan token", err.Error())
		return
	}

	idToken, ok := token.Extra("id_token").(string)
	if !ok {
		utils.ErrorResponse(c, "Tidak mendapatkan id_token dari Google")
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
		utils.ErrorResponseWithDetails(c, "Gagal memproses autentikasi", err.Error())
		return
	}

	// Buat token internal JWT untuk aplikasimu
	internalToken, err := h.authService.GenerateJWT(user)
	if err != nil {
		utils.ErrorResponse(c, "Gagal membuat token internal")
		return
	}

	message := "Login berhasil"
	statusCode := http.StatusOK
	if isNewUser {
		message = "Registrasi berhasil"
		statusCode = http.StatusCreated
	}

	utils.SuccessResponseWithStatus(c, statusCode, gin.H{
		"user":  user,
		"token": internalToken,
	}, message)
}

func (h *AuthHandler) Logout(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		utils.FailResponseWithStatus(c, http.StatusBadRequest, "Invalid Authorization header")
		return
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")

	if err := h.authService.Logout(c.Request.Context(), tokenString); err != nil {
		utils.ErrorResponseWithDetails(c, "Failed to process logout", err.Error())
		return
	}

	c.Status(http.StatusNoContent)
}
