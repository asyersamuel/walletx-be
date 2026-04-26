package main

import (
	"os"

	"walletx-be/configs"
	"walletx-be/pkg/database"
	"walletx-be/pkg/handlers"
	"walletx-be/pkg/repository"
	"walletx-be/pkg/router"
	"walletx-be/pkg/services"

	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

func init() {
	// Standardize global logging format
	logrus.SetFormatter(&logrus.JSONFormatter{})
	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.InfoLevel)
}

func main() {
	if err := godotenv.Load(); err != nil {
		logrus.Warn("⚠️ File .env tidak ditemukan, menggunakan variabel sistem default")
	}

	// 1. Load application configuration
	cfg := configs.Load()

	// 2. Initialize database connection
	logrus.Info("🚀 Initializing database connection...")
	db, err := database.Init(cfg.Database)
	if err != nil {
		logrus.WithError(err).Fatal("❌ Failed to connect to database")
	}
	logrus.Info("✅ Database connected and models migrated successfully")

	// 3. Initialize Repositories
	userRepo        := repository.NewUserRepository(db)
	transactionRepo := repository.NewTransactionRepository(db)
	categoryRepo    := repository.NewCategoryRepository(db)
	budgetRepo      := repository.NewBudgetRepository(db)
	recurringRepo   := repository.NewRecurringRepository(db)
	summaryRepo     := repository.NewSummaryRepository(db)

	// 4. Initialize Services
	authService      := services.NewAuthService(userRepo, cfg)
	parserService    := services.NewParserService()
	txService        := services.NewTransactionService(userRepo, transactionRepo, categoryRepo, parserService)
	categoryService  := services.NewCategoryService(categoryRepo)
	budgetService    := services.NewBudgetService(budgetRepo, summaryRepo, db)
	recurringService := services.NewRecurringService(recurringRepo, transactionRepo)
	dashboardService := services.NewDashboardService(budgetRepo, summaryRepo)

	// 5. Initialize Handlers
	authHandler      := handlers.NewAuthHandler(authService, cfg)
	txHandler        := handlers.NewTransactionHandler(txService)
	categoryHandler  := handlers.NewCategoryHandler(categoryService)
	budgetHandler    := handlers.NewBudgetHandler(budgetService)
	recurringHandler := handlers.NewRecurringHandler(recurringService)
	dashboardHandler := handlers.NewDashboardHandler(dashboardService)

	// 6. Initialize Services for HTTP-triggered Cron tasks
	imapService := services.NewIMAPService(cfg.IMAP.Email, cfg.IMAP.Password, txService)
	cronHandler := handlers.NewCronHandler(db, imapService, recurringService)

	// 8. Setup Router Gin
	logrus.Info("🌐 Starting REST API Server...")
	r := router.SetupRouter(
		txHandler,
		authHandler,
		categoryHandler,
		budgetHandler,
		recurringHandler,
		dashboardHandler,
		cronHandler,
		cfg.JWT.Secret,
		cfg,
		db,
	)

	// 9. Run Server
	port := cfg.Server.Port
	if port == "" {
		port = "8080"
	}

	logrus.WithFields(logrus.Fields{
		"port": port,
		"env":  os.Getenv("GIN_MODE"),
		"url":  "http://localhost:" + port,
	}).Info("🚀 WalletX REST API Server is starting to listen")

	if err := r.Run(":" + port); err != nil {
		logrus.WithError(err).Fatal("❌ Failed to start server")
	}
}

