package app

import (
	"walletx-be/configs"
	"walletx-be/internal/middleware"
	"walletx-be/internal/modules/auth"
	"walletx-be/internal/shared/response"

	"github.com/gin-gonic/gin"
)

// SetupRouter builds the Gin engine and registers the enabled routes.
func SetupRouter(modules *Modules, cfg *configs.Config, validator middleware.TokenValidator) *gin.Engine {
	r := gin.Default()

	r.Use(middleware.CORS())

	api := r.Group("/api/v1")
	{
		api.GET("/ping", func(c *gin.Context) {
			response.Success(c, nil, "API is running")
		})

		auth.RegisterRoutes(api, modules.Auth.Handler, cfg.App.DevMode)

		protected := api.Group("")
		protected.Use(middleware.AuthMiddleware(validator))
		{
			auth.RegisterProtectedRoutes(protected, modules.Auth.Handler)
		}
	}

	return r
}
