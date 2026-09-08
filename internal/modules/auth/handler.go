package auth

import (
	"net/http"
	"strings"

	"walletx-be/internal/shared/response"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
)

type Handler struct {
	service    Service
	oauthConfig *oauth2.Config
}

func NewHandler(service Service, oauthConfig *oauth2.Config) *Handler {
	return &Handler{
		service:    service,
		oauthConfig: oauthConfig,
	}
}

func (h *Handler) HandleGoogleAuth(c *gin.Context) {
	var input GoogleAuthInput

	if err := c.ShouldBindJSON(&input); err != nil {
		response.FailWithDetails(c, "Invalid request body", err.Error())
		return
	}

	h.processAuthLogic(c, input)
}

func (h *Handler) GoogleLoginTest(c *gin.Context) {
	state := "random-state-string"
	url := h.oauthConfig.AuthCodeURL(state)
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func (h *Handler) GoogleCallbackTest(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		response.Fail(c, "Code not provided by Google")
		return
	}

	token, err := h.oauthConfig.Exchange(c.Request.Context(), code)
	if err != nil {
		response.ErrorWithDetails(c, "Failed to exchange code for token", err.Error())
		return
	}

	idToken, ok := token.Extra("id_token").(string)
	if !ok {
		response.Error(c, "id_token not received from Google")
		return
	}

	input := GoogleAuthInput{
		IDToken: idToken,
	}

	h.processAuthLogic(c, input)
}

func (h *Handler) processAuthLogic(c *gin.Context, input GoogleAuthInput) {
	user, isNewUser, err := h.service.ProcessGoogleAuth(c.Request.Context(), input)
	if err != nil {
		response.ErrorWithDetails(c, "Failed to process authentication", err.Error())
		return
	}

	internalToken, err := h.service.GenerateJWT(user)
	if err != nil {
		response.Error(c, "Failed to generate internal token")
		return
	}

	message := "Login successful"
	statusCode := http.StatusOK
	if isNewUser {
		message = "Registration successful"
		statusCode = http.StatusCreated
	}

	response.SuccessWithStatus(c, statusCode, gin.H{
		"user":  user,
		"token": internalToken,
	}, message)
}

func (h *Handler) Logout(c *gin.Context) {
	authHeader := c.GetHeader("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		response.FailWithStatus(c, http.StatusBadRequest, "Invalid Authorization header")
		return
	}

	tokenString := strings.TrimPrefix(authHeader, "Bearer ")

	if err := h.service.Logout(c.Request.Context(), tokenString); err != nil {
		response.ErrorWithDetails(c, "Failed to process logout", err.Error())
		return
	}

	c.Status(http.StatusNoContent)
}
