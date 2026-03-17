package main

import (
	"os"

	"walletx-be/configs"
	"walletx-be/internal/database"
	"walletx-be/internal/handlers"
	"walletx-be/internal/repository"
	"walletx-be/internal/router"
	"walletx-be/internal/services"
	"walletx-be/internal/workers"

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
	txService        := services.NewTransactionService(userRepo, transactionRepo, parserService)
	categoryService  := services.NewCategoryService(categoryRepo)
	budgetService    := services.NewBudgetService(budgetRepo)
	recurringService := services.NewRecurringService(recurringRepo)
	dashboardService := services.NewDashboardService(budgetRepo, summaryRepo)

	// 5. Initialize Handlers
	authHandler      := handlers.NewAuthHandler(authService, cfg)
	txHandler        := handlers.NewTransactionHandler(txService)
	categoryHandler  := handlers.NewCategoryHandler(categoryService)
	budgetHandler    := handlers.NewBudgetHandler(budgetService)
	recurringHandler := handlers.NewRecurringHandler(recurringService)
	dashboardHandler := handlers.NewDashboardHandler(dashboardService)

	// 6. Initialize Workers & Cron
	imapWorker      := workers.NewIMAPWorker(cfg.IMAP.Email, cfg.IMAP.Password, txService)
	recurringWorker := workers.NewRecurringWorker(recurringRepo, transactionRepo)
	cronScheduler   := workers.SetupCronJobs(imapWorker, recurringWorker)

	// Start the cron scheduler in a non-blocking way
	cronScheduler.Start()
	logrus.Info("🛡️ [System] Background scheduler started successfully")

	// Pastikan cron dimatikan saat aplikasi berhenti (Graceful Shutdown)
	defer func() {
		logrus.Info("🛑 [System] Stopping background scheduler...")
		cronScheduler.Stop()
	}()

	// Development - run IMAP worker once at startup for testing
	logrus.Info("🛠️ [Dev Mode] Menjalankan IMAP Worker satu kali saat startup...")
	_ = imapWorker.ProcessUnseenEmails()

	// 7. Setup Router Gin
	logrus.Info("🌐 Starting REST API Server...")
	r := router.SetupRouter(
		txHandler,
		authHandler,
		categoryHandler,
		budgetHandler,
		recurringHandler,
		dashboardHandler,
		cfg.JWT.Secret,
		cfg,
	)

	// 8. Run Server
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

