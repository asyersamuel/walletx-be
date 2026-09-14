package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"walletx-be/internal/platform/logger"
	apperrors "walletx-be/internal/shared/errors"
	"walletx-be/internal/shared/response"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
)

type Handler struct {
	service     Service
	logger      logger.Logger
	oauthConfig *oauth2.Config
}

func NewHandler(service Service, appLogger logger.Logger, oauthConfig *oauth2.Config) *Handler {
	return &Handler{
		service:     service,
		logger:      appLogger,
		oauthConfig: oauthConfig,
	}
}

func (h *Handler) HandleGoogleAuth(c *gin.Context) {
	var input GoogleAuthInput

	if c.Request.Body == nil {
		response.Fail(c, "Request body is required")
		return
	}

	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		response.Fail(c, "Invalid request body")
		return
	}

	if strings.TrimSpace(input.IDToken) == "" {
		response.Fail(c, "id_token is required")
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
		switch {
		case errors.Is(err, apperrors.ErrInvalidInput):
			h.logger.WithField("operation", "google_auth").Warn("Invalid authentication request")
			response.Fail(c, "Invalid authentication request")
		case errors.Is(err, apperrors.ErrUnauthorized):
			h.logger.WithField("operation", "google_auth").Warn("Google token validation failed")
			response.Unauthorized(c, "Invalid or expired Google token")
		case errors.Is(err, apperrors.ErrConflict), errors.Is(err, apperrors.ErrAccountDeleted), errors.Is(err, apperrors.ErrDuplicate):
			h.logger.WithField("operation", "google_auth").Warn("Account conflict during authentication")
			response.FailWithStatus(c, http.StatusConflict, "Account conflict")
		default:
			h.logger.WithError(err).WithField("operation", "google_auth").Error("Authentication failed")
			response.Error(c, "Failed to process authentication")
		}
		return
	}

	internalToken, err := h.service.GenerateJWT(user)
	if err != nil {
		h.logger.WithError(err).WithField("operation", "jwt_generation").Error("Failed to generate internal JWT")
		response.Error(c, "Failed to generate internal token")
		return
	}

	message := "Login successful"
	statusCode := http.StatusOK
	if isNewUser {
		message = "Registration successful"
		statusCode = http.StatusCreated
	}

	response.SuccessWithStatus(c, statusCode, GoogleAuthResponse{
		User:  toUserResponse(user),
		Token: internalToken,
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
