package router

import (
	"time"

	config "walletx-be/configs"
	"walletx-be/pkg/handlers"
	"walletx-be/pkg/middleware"
	"walletx-be/pkg/repository"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func SetupRouter(
	txHandler         *handlers.TransactionHandler,
	authHandler       *handlers.AuthHandler,
	categoryHandler   *handlers.CategoryHandler,
	budgetHandler     *handlers.BudgetHandler,
	recurringHandler  *handlers.RecurringHandler,
	dashboardHandler  *handlers.DashboardHandler,
	cronHandler       *handlers.CronHandler,
	jwtSecret         string,
	cfg               *config.Config,
	db                *gorm.DB,
	blacklistRepo     repository.TokenBlacklistRepository,
) *gin.Engine {
	r := gin.Default()

	// CORS Configuration
	r.Use(cors.New(cors.Config{
		AllowAllOrigins:  true,
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// Serve static files only for local storage
	if cfg.Media.StorageType == "local" {
		r.Static(cfg.Media.BaseURL, cfg.Media.UploadDir)
	}

	// Setup API Routes
	api := r.Group("/api/v1")
	{
		// Health check
		api.GET("/ping", func(c *gin.Context) {
			c.JSON(200, gin.H{"message": "WalletX API is running!"})
		})

		// ── Auth Routes (public) ─────────────────────────────────────────────
		auth := api.Group("/auth")
		{
			auth.POST("/google", authHandler.HandleGoogleAuth)

			if cfg.App.DevMode {
				auth.GET("/google/test-login", authHandler.GoogleLoginTest)
				auth.GET("/google/callback", authHandler.GoogleCallbackTest)
			}
		}

		// ── Protected Routes (require valid JWT) ─────────────────────────────
		protected := api.Group("")
		protected.Use(middleware.AuthMiddleware(jwtSecret, blacklistRepo))
		{
			// Logout endpoint
			authProtected := protected.Group("/auth")
			{
				authProtected.POST("/logout", authHandler.Logout)
			}
			// Transactions
			txGroup := protected.Group("/transactions")
			{
				txGroup.GET("", txHandler.GetUserTransactions)
				txGroup.POST("", txHandler.CreateTransaction)
				txGroup.PUT("/:id", txHandler.UpdateTransaction)
				txGroup.DELETE("/:id", txHandler.DeleteTransaction)
				txGroup.GET("/search", txHandler.SearchTransactions)
				txGroup.GET("/export", txHandler.ExportCSV)
			}

			// Categories
			catGroup := protected.Group("/categories")
			{
				catGroup.GET("", categoryHandler.ListCategories)
				catGroup.POST("", categoryHandler.CreateCategory)
				catGroup.GET("/:id", categoryHandler.GetCategoryByID)
				catGroup.PUT("/:id", categoryHandler.UpdateCategory)
				catGroup.DELETE("/:id", categoryHandler.DeleteCategory)
			}

			// Budget Limits
			budgetGroup := protected.Group("/budgets")
			{
				budgetGroup.GET("", budgetHandler.ListBudgets)
				budgetGroup.GET("/progress", budgetHandler.GetBudgetProgress)
				budgetGroup.POST("", budgetHandler.CreateBudget)
				budgetGroup.GET("/:id", budgetHandler.GetBudgetByID)
				budgetGroup.PUT("/:id", budgetHandler.UpdateBudget)
				budgetGroup.DELETE("/:id", budgetHandler.DeleteBudget)
			}

			// Recurring Configs
			recurringGroup := protected.Group("/recurrings")
			{
				recurringGroup.GET("", recurringHandler.ListRecurrings)
				recurringGroup.POST("", recurringHandler.CreateRecurring)
				recurringGroup.GET("/:id", recurringHandler.GetRecurringByID)
				recurringGroup.PUT("/:id", recurringHandler.UpdateRecurring)
				recurringGroup.DELETE("/:id", recurringHandler.DeleteRecurring)
			}

			// Dashboard
			dashGroup := protected.Group("/dashboard")
			{
				dashGroup.GET("/budget-summary", dashboardHandler.GetBudgetSummary)
				dashGroup.GET("/calendar", dashboardHandler.GetDailyCalendar)
			}

			reportGroup := protected.Group("/reports")
			{
				reportGroup.GET("/expenses-by-category", txHandler.GetReports)
			}
		}

		// ── Cron Routes (protected by CronAuthMiddleware) ────────────────────
		cronGroup := api.Group("/cron")
		cronGroup.Use(middleware.CronAuthMiddleware())
		{
			cronGroup.GET("/keep-alive", cronHandler.KeepAlive)
			cronGroup.POST("/imap", cronHandler.IMAPSync)
			cronGroup.POST("/recurring", cronHandler.RecurringSync)
		}
	}

	return r
}
