package router

import (
	"time"

	config "walletx-be/configs"
	"walletx-be/pkg/handlers"
	"walletx-be/pkg/middleware"
	"walletx-be/pkg/repository"
	"walletx-be/pkg/utils"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SetupRouter configures all application routes with proper middleware
// 
// RESTful API Design Principles Applied:
// - Resource-based naming (plural nouns): /transactions, /categories, /budgets
// - HTTP method semantics: GET (read), POST (create), PUT (update), DELETE (remove)
// - Proper status codes: 200 OK, 201 Created, 204 No Content, 400 Bad Request, 404 Not Found, 500 Server Error
// - Consistent response envelope: { status, message, data, meta, timestamp }
// - Query parameters for filtering, sorting, pagination
// - Content negotiation for CSV export (Accept: text/csv)
//
// Route Groups:
// - /api/v1 (base)
//   - /auth (public) - Google OAuth login
//   - protected (JWT required) - All CRUD operations and reports
//   - /cron (secret-based auth) - Background job triggers (Vercel cron compatible)
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
			utils.SuccessResponse(c, nil, "WalletX API is running!")
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
		// All routes under this group are protected by JWT authentication middleware
		protected := api.Group("")
		protected.Use(middleware.AuthMiddleware(jwtSecret, blacklistRepo))
		{
			// Auth endpoints
			authProtected := protected.Group("/auth")
			{
				authProtected.POST("/logout", authHandler.Logout)
			}

			// Transactions - Full CRUD with filtering, sorting, pagination
			// GET    /transactions          - List with filters (?q=, ?date_from=, ?amount_min=, ?sort=)
			// POST   /transactions          - Create new transaction
			// GET    /transactions/:id      - Get single transaction (via generic handler if needed)
			// PUT    /transactions/:id      - Update transaction (partial update supported)
			// DELETE /transactions/:id      - Delete transaction (returns 204 No Content)
			txGroup := protected.Group("/transactions")
			{
				txGroup.GET("", txHandler.GetUserTransactions)
				txGroup.POST("", txHandler.CreateTransaction)
				txGroup.PUT("/:id", txHandler.UpdateTransaction)
				txGroup.DELETE("/:id", txHandler.DeleteTransaction)
			}

			// Categories - Full CRUD for user-defined categories
			catGroup := protected.Group("/categories")
			{
				catGroup.GET("", categoryHandler.ListCategories)
				catGroup.POST("", categoryHandler.CreateCategory)
				catGroup.GET("/:id", categoryHandler.GetCategoryByID)
				catGroup.PUT("/:id", categoryHandler.UpdateCategory)
				catGroup.DELETE("/:id", categoryHandler.DeleteCategory)
			}

			// Budgets - Budget limit management with progress tracking
			// GET    /budgets          - List all budgets
			// GET    /budgets/progress - Get budget vs actual spending progress
			// POST   /budgets          - Create new budget
			// GET    /budgets/:id      - Get single budget
			// PUT    /budgets/:id      - Update budget
			// DELETE /budgets/:id      - Delete budget
			budgetGroup := protected.Group("/budgets")
			{
				budgetGroup.GET("", budgetHandler.ListBudgets)
				budgetGroup.GET("/progress", budgetHandler.GetBudgetProgress)
				budgetGroup.POST("", budgetHandler.CreateBudget)
				budgetGroup.GET("/:id", budgetHandler.GetBudgetByID)
				budgetGroup.PUT("/:id", budgetHandler.UpdateBudget)
				budgetGroup.DELETE("/:id", budgetHandler.DeleteBudget)
			}

			// Recurring Transactions - Recurring transaction configuration
			recurringGroup := protected.Group("/recurrings")
			{
				recurringGroup.GET("", recurringHandler.ListRecurrings)
				recurringGroup.POST("", recurringHandler.CreateRecurring)
				recurringGroup.GET("/:id", recurringHandler.GetRecurringByID)
				recurringGroup.PUT("/:id", recurringHandler.UpdateRecurring)
				recurringGroup.DELETE("/:id", recurringHandler.DeleteRecurring)
			}

			// Reports - Analytics and reporting endpoints (moved from /dashboard)
			// GET /reports/expenses         - Expenses grouped by category
			// GET /reports/budget-summary   - Budget summary with actual spending
			// GET /reports/daily-calendar   - Daily spending calendar for month/year
			reportGroup := protected.Group("/reports")
			{
				reportGroup.GET("/expenses", txHandler.GetReports)
				reportGroup.GET("/budget-summary", dashboardHandler.GetBudgetSummary)
				reportGroup.GET("/daily-calendar", dashboardHandler.GetDailyCalendar)
			}
		}

		// ── Cron Routes (protected by CronAuthMiddleware) ────────────────────
		cronGroup := api.Group("/cron")
		cronGroup.Use(middleware.CronAuthMiddleware())
		{
			cronGroup.GET("/keep-alive", cronHandler.KeepAlive)
			cronGroup.GET("/imap", cronHandler.IMAPSync)
			cronGroup.GET("/recurring", cronHandler.RecurringSync)
		}
	}

	return r
}
