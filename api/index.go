package api

import (
	"net/http"
	"os"

	"walletx-be/configs"
	"walletx-be/pkg/cache"
	"walletx-be/pkg/database"
	"walletx-be/pkg/handlers"
	"walletx-be/pkg/repository"
	"walletx-be/pkg/router"
	"walletx-be/pkg/services"

	"github.com/joho/godotenv"
	"github.com/sirupsen/logrus"
)

var app http.Handler

func init() {
	// Standardize global logging format
	logrus.SetFormatter(&logrus.JSONFormatter{})
	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.InfoLevel)

	if err := godotenv.Load(); err != nil {
		logrus.Warn("⚠️ File .env tidak ditemukan, menggunakan variabel sistem default")
	}

	cfg := configs.Load()

	db, err := database.Init(cfg.Database)
	if err != nil {
		logrus.WithError(err).Fatal("❌ Failed to connect to database in Vercel Cold Start")
	}

	// Initialize Redis connection for token blacklisting
	redisClient, err := cache.InitRedis(cfg.Redis)
	if err != nil {
		logrus.WithError(err).Warn("⚠️ Redis not connected, token blacklisting disabled")
	}

	var blacklistRepo repository.TokenBlacklistRepository
	if redisClient != nil {
		blacklistRepo = repository.NewTokenBlacklistRepository(redisClient)
	} else {
		blacklistRepo = repository.NewNoOpBlacklistRepository()
	}

	userRepo := repository.NewUserRepository(db)
	transactionRepo := repository.NewTransactionRepository(db)
	categoryRepo := repository.NewCategoryRepository(db)
	budgetRepo := repository.NewBudgetRepository(db)
	recurringRepo := repository.NewRecurringRepository(db)
	summaryRepo := repository.NewSummaryRepository(db)

	authService := services.NewAuthService(userRepo, cfg, blacklistRepo)
	parserService := services.NewParserService()
	txService := services.NewTransactionService(userRepo, transactionRepo, categoryRepo, parserService)
	categoryService := services.NewCategoryService(categoryRepo)
	budgetService := services.NewBudgetService(budgetRepo, summaryRepo, db)
	// Passing transactionRepo here as agreed in the plan
	recurringService := services.NewRecurringService(recurringRepo, transactionRepo)
	dashboardService := services.NewDashboardService(budgetRepo, summaryRepo)

	authHandler := handlers.NewAuthHandler(authService, cfg)
	txHandler := handlers.NewTransactionHandler(txService)
	categoryHandler := handlers.NewCategoryHandler(categoryService)
	budgetHandler := handlers.NewBudgetHandler(budgetService)
	recurringHandler := handlers.NewRecurringHandler(recurringService)
	dashboardHandler := handlers.NewDashboardHandler(dashboardService)

	// Services for Cron tasks
	imapService := services.NewIMAPService(cfg.IMAP.Email, cfg.IMAP.Password, txService)
	cronHandler := handlers.NewCronHandler(db, imapService, recurringService)

	// In Serverless mode, we don't start the background worker. HTTP handles cron logic.

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
		blacklistRepo,
	)

	app = r
}

func Handler(w http.ResponseWriter, r *http.Request) {
	app.ServeHTTP(w, r)
}
