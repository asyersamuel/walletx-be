package handlers

import (
	"net/http"
	"strings"

	"walletx-be/pkg/services"
	"walletx-be/pkg/utils"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
)

type AuthHandler struct {
	authService services.AuthService
	oauthConfig *oauth2.Config
}

func NewAuthHandler(authService services.AuthService, oauthConfig *oauth2.Config) *AuthHandler {
	return &AuthHandler{
		authService: authService,
		oauthConfig: oauthConfig,
	}
}

func (h *AuthHandler) HandleGoogleAuth(c *gin.Context) {
	var input services.GoogleAuthInput

	if err := c.ShouldBindJSON(&input); err != nil {
		utils.FailResponseWithDetails(c, "Invalid request body", err.Error())
		return
	}

	h.processAuthLogic(c, input)
}

func (h *AuthHandler) GoogleLoginTest(c *gin.Context) {
	state := "random-state-string"
	url := h.oauthConfig.AuthCodeURL(state)
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func (h *AuthHandler) GoogleCallbackTest(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		utils.FailResponse(c, "Code not provided by Google")
		return
	}

	token, err := h.oauthConfig.Exchange(c.Request.Context(), code)
	if err != nil {
		utils.ErrorResponseWithDetails(c, "Failed to exchange code for token", err.Error())
		return
	}

	idToken, ok := token.Extra("id_token").(string)
	if !ok {
		utils.ErrorResponse(c, "id_token not received from Google")
		return
	}

	input := services.GoogleAuthInput{
		IDToken: idToken,
	}

	h.processAuthLogic(c, input)
}

func (h *AuthHandler) processAuthLogic(c *gin.Context, input services.GoogleAuthInput) {
	user, isNewUser, err := h.authService.ProcessGoogleAuth(c.Request.Context(), input)
	if err != nil {
		utils.ErrorResponseWithDetails(c, "Failed to process authentication", err.Error())
		return
	}

	internalToken, err := h.authService.GenerateJWT(user)
	if err != nil {
		utils.ErrorResponse(c, "Failed to generate internal token")
		return
	}

	message := "Login successful"
	statusCode := http.StatusOK
	if isNewUser {
		message = "Registration successful"
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
