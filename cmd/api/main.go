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
	"github.com/domibowo/booking-systems/internal/booking"
	"github.com/domibowo/booking-systems/internal/catalog"
	"github.com/domibowo/booking-systems/internal/health"
	"github.com/domibowo/booking-systems/internal/platform/config"
	"github.com/domibowo/booking-systems/internal/platform/database"
	"github.com/domibowo/booking-systems/internal/platform/httpserver"
	"github.com/domibowo/booking-systems/internal/schedule"

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
		&schedule.WeeklyHours{}, &schedule.ScheduleException{}, &schedule.ScheduleSettings{},
		&booking.Booking{},
	); err != nil {
		slog.Error("database migration failed", "error", err)
		os.Exit(1)
	}

	if err := booking.EnsureSchema(db); err != nil {
		slog.Error("booking schema setup failed", "error", err)
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

	catalogService := catalog.NewCatalog(
		catalog.NewGormBusinessRepository(db),
		catalog.NewGormResourceRepository(db),
		catalog.NewGormServiceRepository(db),
	)
	catalogHandler := catalog.NewHandler(catalogService)
	catalogHandler.RegisterRoutes(router, authHandler.RequireAuth(), auth.RequireRole(auth.RoleOwner, auth.RoleSuperadmin))

	scheduleService := schedule.NewSchedule(
		schedule.NewGormWeeklyHoursRepository(db),
		schedule.NewGormExceptionRepository(db),
		schedule.NewGormSettingsRepository(db),
		catalogService,
		catalogService,
		catalogService,
	)
	scheduleHandler := schedule.NewHandler(scheduleService)
	scheduleHandler.RegisterRoutes(router, authHandler.RequireAuth(), auth.RequireRole(auth.RoleOwner, auth.RoleSuperadmin))

	bookingHandler := booking.NewHandler(booking.NewBookings(
		booking.NewGormRepository(db),
		catalogService,
		catalogService,
		catalogService,
		catalogService,
		scheduleService,
		cfg.BookingHoldDuration,
	))
	bookingHandler.RegisterRoutes(router, authHandler.RequireAuth(), auth.RequireRole(auth.RoleOwner, auth.RoleSuperadmin))

	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := httpserver.Run(ctx, ":"+cfg.AppPort, router); err != nil {
		slog.Error("server stopped with error", "error", err)
		os.Exit(1)
	}

	slog.Info("server stopped gracefully")
}
