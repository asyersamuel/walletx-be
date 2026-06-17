package router

import (
	"walletx-be/pkg/handlers"
	"walletx-be/pkg/middleware"

	"github.com/gin-gonic/gin"
)

func SetupRouter(
	txHandler        *handlers.TransactionHandler,
	authHandler      *handlers.AuthHandler,
	categoryHandler  *handlers.CategoryHandler,
	budgetHandler    *handlers.BudgetHandler,
	recurringHandler *handlers.RecurringHandler,
	dashboardHandler *handlers.DashboardHandler,
	cronHandler      *handlers.CronHandler,
	telegramHandler  *handlers.TelegramHandler,
	userHandler      *handlers.UserHandler,
	jwtSecret        string,
	devMode          bool,
	storageType      string,
	baseURL          string,
	uploadDir        string,
	blacklistRepo    middleware.TokenBlacklistRepository,
	cronSecret       string,
) *gin.Engine {
	r := gin.Default()

	r.Use(SetupCORS())

	SetupStaticFileServing(r, storageType, baseURL, uploadDir)

	validator := middleware.NewJWTValidator(jwtSecret, blacklistRepo)

	api := r.Group("/api/v1")
	{
		RegisterHealthCheck(api)

		// ── Auth Routes (public) ─────────────────────────────────────────────
		RegisterAuthRoutes(api, authHandler, devMode)

		// ── Protected Routes (require valid JWT) ─────────────────────────────
		// All routes under this group are protected by JWT authentication middleware
		protected := api.Group("")
		protected.Use(middleware.AuthMiddleware(validator))
		{
			RegisterAuthLogoutRoutes(protected, authHandler)
			RegisterUserRoutes(protected, userHandler)
			RegisterTransactionRoutes(protected, txHandler)
			RegisterCategoryRoutes(protected, categoryHandler)
			RegisterBudgetRoutes(protected, budgetHandler)
			RegisterRecurringRoutes(protected, recurringHandler)
			RegisterReportRoutes(protected, txHandler, dashboardHandler)
		}

		// ── Cron Routes ───────────────────────────────────────────────────────
		RegisterCronRoutes(api, cronHandler, cronSecret)

		// ── Telegram Routes (public – called by Telegram servers & email links)
		RegisterTelegramRoutes(api, telegramHandler)
	}

	return r
}
