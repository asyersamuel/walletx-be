package app

import (
	"walletx-be/configs"
	"walletx-be/internal/middleware"
	"walletx-be/internal/modules/auth"
	"walletx-be/internal/modules/budget"
	"walletx-be/internal/modules/category"
	"walletx-be/internal/modules/cron"
	"walletx-be/internal/modules/dashboard"
	"walletx-be/internal/modules/recurring"
	"walletx-be/internal/modules/telegram"
	"walletx-be/internal/modules/transaction"
	"walletx-be/internal/shared/response"

	"github.com/gin-gonic/gin"
)

// SetupRouter builds the Gin engine, registers cross-cutting middleware and
// delegates route registration to each module.
func SetupRouter(modules *Modules, cfg *configs.Config, validator middleware.TokenValidator) *gin.Engine {
	r := gin.Default()

	r.Use(middleware.CORS())

	setupStaticFileServing(r, cfg.Media.StorageType, cfg.Media.BaseURL, cfg.Media.UploadDir)

	api := r.Group("/api/v1")
	{
		api.GET("/ping", func(c *gin.Context) {
			response.Success(c, nil, "WalletX API is running!")
		})

		// ── Public auth routes ─────────────────────────────────────────────
		auth.RegisterRoutes(api, modules.Auth.Handler, cfg.App.DevMode)

		// ── Protected routes (require valid JWT) ───────────────────────────
		protected := api.Group("")
		protected.Use(middleware.AuthMiddleware(validator))
		{
			auth.RegisterProtectedRoutes(protected, modules.Auth.Handler)
			transaction.RegisterRoutes(protected, modules.Transaction.Handler)
			category.RegisterRoutes(protected, modules.Category.Handler)
			budget.RegisterRoutes(protected, modules.Budget.Handler)
			recurring.RegisterRoutes(protected, modules.Recurring.Handler)
			transaction.RegisterReportRoutes(protected, modules.Transaction.Handler)
			dashboard.RegisterReportRoutes(protected, modules.Dashboard.Handler)
		}

		// ── Cron routes ───────────────────────────────────────────────────
		cron.RegisterRoutes(api, modules.Cron.Handler, cfg.Cron.Secret)

		// ── Telegram routes (public) ──────────────────────────────────────
		telegram.RegisterRoutes(api, modules.Telegram.Handler)
	}

	return r
}

func setupStaticFileServing(r *gin.Engine, storageType, baseURL, uploadDir string) {
	if storageType == "local" {
		r.Static(baseURL, uploadDir)
	}
}
