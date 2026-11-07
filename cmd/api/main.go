package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/domibowo/booking-systems/internal/auth"
	"github.com/domibowo/booking-systems/internal/catalog"
	"github.com/domibowo/booking-systems/internal/health"
	"github.com/domibowo/booking-systems/internal/platform/config"
	"github.com/domibowo/booking-systems/internal/platform/database"
	"github.com/domibowo/booking-systems/internal/platform/httpserver"

	_ "github.com/domibowo/booking-systems/docs"
)

// @title Booking Systems API
// @version 1.0
// @description Backend REST API for a booking system — portfolio project and a refresher on Go backend architecture.
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and the access token, e.g. "Bearer eyJhbGci...".
func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("config load failed", "error", err)
		os.Exit(1)
	}

	db, err := database.New(cfg.DatabaseURL)
	if err != nil {
		slog.Error("database connection failed", "error", err)
		os.Exit(1)
	}

	if err := db.AutoMigrate(
		&auth.User{}, &auth.RefreshToken{},
		&catalog.Business{}, &catalog.Resource{}, &catalog.Service{},
	); err != nil {
		slog.Error("database migration failed", "error", err)
		os.Exit(1)
	}

	router := httpserver.NewRouter(cfg.GinMode)

	health.NewHandler(db).RegisterRoutes(router)

	authService := auth.NewService(
		auth.NewGormUserRepository(db),
		auth.NewGormRefreshTokenRepository(db),
		auth.TokenConfig{
			Secret:          []byte(cfg.JWTSecret),
			AccessTokenTTL:  cfg.AccessTokenTTL,
			RefreshTokenTTL: cfg.RefreshTokenTTL,
		},
	)
	authHandler := auth.NewHandler(authService)
	authHandler.RegisterRoutes(router)

	catalogHandler := catalog.NewHandler(catalog.NewCatalog(
		catalog.NewGormBusinessRepository(db),
		catalog.NewGormResourceRepository(db),
		catalog.NewGormServiceRepository(db),
	))
	catalogHandler.RegisterRoutes(router, authHandler.RequireAuth(), auth.RequireRole(auth.RoleOwner, auth.RoleSuperadmin))

	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := httpserver.Run(ctx, ":"+cfg.AppPort, router); err != nil {
		slog.Error("server stopped with error", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped gracefully")
}
