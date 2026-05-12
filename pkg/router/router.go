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
	webhookHandler   *handlers.WebhookHandler,
	jwtSecret        string,
	devMode          bool,
	storageType      string,
	baseURL          string,
	uploadDir        string,
	blacklistRepo    middleware.TokenBlacklistRepository,
	cronSecret       string,
	qstashSigningKey string,
) *gin.Engine {
	r := gin.Default()

	r.Use(SetupCORS())

	SetupStaticFileServing(r, storageType, baseURL, uploadDir)

	api := r.Group("/api/v1")
	{
		RegisterHealthCheck(api)

		RegisterAuthRoutes(api, authHandler, devMode)

		validator := middleware.NewJWTValidator(jwtSecret, blacklistRepo)
		protected := api.Group("")
		protected.Use(middleware.AuthMiddleware(validator))
		{
			RegisterAuthLogoutRoutes(protected, authHandler)
			RegisterTransactionRoutes(protected, txHandler)
			RegisterCategoryRoutes(protected, categoryHandler)
			RegisterBudgetRoutes(protected, budgetHandler)
			RegisterRecurringRoutes(protected, recurringHandler)
			RegisterReportRoutes(protected, txHandler, dashboardHandler)
		}

		RegisterCronRoutes(api, cronHandler, cronSecret)
		RegisterWebhookRoutes(api, webhookHandler, qstashSigningKey)
	}

	return r
}
