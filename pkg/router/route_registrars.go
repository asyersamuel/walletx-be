package router

import (
	"walletx-be/pkg/handlers"
	"walletx-be/pkg/middleware"

	"github.com/gin-gonic/gin"
)

func RegisterAuthRoutes(r *gin.RouterGroup, authHandler *handlers.AuthHandler, devMode bool) {
	auth := r.Group("/auth")
	{
		auth.POST("/google", authHandler.HandleGoogleAuth)

		if devMode {
			auth.GET("/google/test-login", authHandler.GoogleLoginTest)
			auth.GET("/google/callback", authHandler.GoogleCallbackTest)
		}
	}
}

func RegisterAuthLogoutRoutes(r *gin.RouterGroup, authHandler *handlers.AuthHandler) {
	authProtected := r.Group("/auth")
	{
		authProtected.POST("/logout", authHandler.Logout)
	}
}

func RegisterTransactionRoutes(r *gin.RouterGroup, txHandler *handlers.TransactionHandler) {
	txGroup := r.Group("/transactions")
	{
		txGroup.GET("", txHandler.GetUserTransactions)
		txGroup.POST("", txHandler.CreateTransaction)
		txGroup.PUT("/:id", txHandler.UpdateTransaction)
		txGroup.DELETE("/:id", txHandler.DeleteTransaction)
	}
}

func RegisterCategoryRoutes(r *gin.RouterGroup, categoryHandler *handlers.CategoryHandler) {
	catGroup := r.Group("/categories")
	{
		catGroup.GET("", categoryHandler.ListCategories)
		catGroup.POST("", categoryHandler.CreateCategory)
		catGroup.GET("/:id", categoryHandler.GetCategoryByID)
		catGroup.PUT("/:id", categoryHandler.UpdateCategory)
		catGroup.DELETE("/:id", categoryHandler.DeleteCategory)
	}
}

func RegisterBudgetRoutes(r *gin.RouterGroup, budgetHandler *handlers.BudgetHandler) {
	budgetGroup := r.Group("/budgets")
	{
		budgetGroup.GET("", budgetHandler.ListBudgets)
		budgetGroup.GET("/progress", budgetHandler.GetBudgetProgress)
		budgetGroup.POST("", budgetHandler.CreateBudget)
		budgetGroup.GET("/:id", budgetHandler.GetBudgetByID)
		budgetGroup.PUT("/:id", budgetHandler.UpdateBudget)
		budgetGroup.DELETE("/:id", budgetHandler.DeleteBudget)
	}
}

func RegisterRecurringRoutes(r *gin.RouterGroup, recurringHandler *handlers.RecurringHandler) {
	recurringGroup := r.Group("/recurrings")
	{
		recurringGroup.GET("", recurringHandler.ListRecurrings)
		recurringGroup.POST("", recurringHandler.CreateRecurring)
		recurringGroup.GET("/:id", recurringHandler.GetRecurringByID)
		recurringGroup.PUT("/:id", recurringHandler.UpdateRecurring)
		recurringGroup.DELETE("/:id", recurringHandler.DeleteRecurring)
	}
}

func RegisterReportRoutes(r *gin.RouterGroup, txHandler *handlers.TransactionHandler, dashboardHandler *handlers.DashboardHandler) {
	reportGroup := r.Group("/reports")
	{
		reportGroup.GET("/expenses", txHandler.GetReports)
		reportGroup.GET("/budget-summary", dashboardHandler.GetBudgetSummary)
		reportGroup.GET("/daily-calendar", dashboardHandler.GetDailyCalendar)
	}
}

func RegisterCronRoutes(r *gin.RouterGroup, cronHandler *handlers.CronHandler, cronSecret string) {
	cronGroup := r.Group("/cron")
	cronGroup.Use(middleware.CronAuthMiddleware(cronSecret))
	{
		cronGroup.GET("/keep-alive", cronHandler.KeepAlive)
		cronGroup.GET("/imap", cronHandler.IMAPSync)
		cronGroup.GET("/recurring", cronHandler.RecurringSync)
	}
}

func RegisterWebhookRoutes(r *gin.RouterGroup, webhookHandler *handlers.WebhookHandler, qstashSigningKey string) {
	webhookGroup := r.Group("/internal/webhooks")
	webhookGroup.Use(middleware.QStashAuthMiddleware(qstashSigningKey))
	{
		webhookGroup.POST("/email-processor", webhookHandler.ProcessEmail)
	}
}
