package bootstrap

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/Collinsthegreat/hng14_stage1_backend/db/migrations"
	"github.com/Collinsthegreat/hng14_stage1_backend/db/seed"
	"github.com/Collinsthegreat/hng14_stage1_backend/internal/client"
	"github.com/Collinsthegreat/hng14_stage1_backend/internal/handler"
	"github.com/Collinsthegreat/hng14_stage1_backend/internal/middleware"
	"github.com/Collinsthegreat/hng14_stage1_backend/internal/repository"
	"github.com/Collinsthegreat/hng14_stage1_backend/internal/service"
	"github.com/Collinsthegreat/hng14_stage1_backend/pkg/response"
	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func NewRouter() http.Handler {
	// Load .env file if it exists (for local development)
	_ = godotenv.Load()

	// ── Step 1: Database connection ───────────────────────────────────────────
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx := context.Background()
	poolConfig, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		log.Fatalf("Invalid DATABASE_URL: %v", err)
	}
	poolConfig.MaxConns = 20
	poolConfig.MinConns = 5
	poolConfig.MaxConnLifetime = 30 * time.Minute
	poolConfig.MaxConnIdleTime = 5 * time.Minute
	poolConfig.HealthCheckPeriod = 1 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v", err)
	}
	slog.Info("Step 1: DB Pool connected")

	// ── Step 2: Migration 001 ─────────────────────────────────────────────────
	if migrations.CreateProfilesSQL != "" {
		if _, err := pool.Exec(ctx, migrations.CreateProfilesSQL); err != nil {
			log.Fatalf("Failed to run migration 001: %v", err)
		}
	}
	slog.Info("Step 2: Migration 001 completed")

	// ── Step 3: Migration 002 ─────────────────────────────────────────────────
	if migrations.AddCountryNameSQL != "" {
		if _, err := pool.Exec(ctx, migrations.AddCountryNameSQL); err != nil {
			log.Fatalf("Failed to run migration 002: %v", err)
		}
	}
	slog.Info("Step 3: Migration 002 completed")

	// ── Step 4: Migration 003 (users + refresh_tokens) ────────────────────────
	if migrations.CreateUsersTokensSQL != "" {
		if _, err := pool.Exec(ctx, migrations.CreateUsersTokensSQL); err != nil {
			log.Fatalf("Failed to run migration 003: %v", err)
		}
	}
	slog.Info("Step 4: Migration 003 completed")

	// ── Step 4B: Migration 004 (performance indexes) ─────────────────────────
	if migrations.PerformanceIndexesSQL != "" {
		if _, err := pool.Exec(ctx, migrations.PerformanceIndexesSQL); err != nil {
			log.Fatalf("Failed to run migration 004: %v", err)
		}
	}
	slog.Info("Step 4B: Migration 004 completed")

	// ── Step 5: Seed profiles ─────────────────────────────────────────────────
	if err := seed.SeedProfiles(ctx, pool); err != nil {
		log.Fatalf("Failed to run seed profiles: %v", err)
	}
	slog.Info("Step 5: SeedProfiles completed")

	// ── Step 6: External HTTP clients ─────────────────────────────────────────
	timeoutStr := os.Getenv("HTTP_TIMEOUT_SECONDS")
	timeoutSecs, err := strconv.Atoi(timeoutStr)
	if err != nil || timeoutSecs <= 0 {
		timeoutSecs = 5
	}
	httpClient := &http.Client{Timeout: time.Duration(timeoutSecs) * time.Second}

	genderizeBase := os.Getenv("GENDERIZE_BASE_URL")
	if genderizeBase == "" {
		genderizeBase = "https://api.genderize.io"
	}
	agifyBase := os.Getenv("AGIFY_BASE_URL")
	if agifyBase == "" {
		agifyBase = "https://api.agify.io"
	}
	natBase := os.Getenv("NATIONALIZE_BASE_URL")
	if natBase == "" {
		natBase = "https://api.nationalize.io"
	}

	genderizeClient := client.NewGenderizeClient(httpClient, genderizeBase)
	agifyClient := client.NewAgifyClient(httpClient, agifyBase)
	natClient := client.NewNationalizeClient(httpClient, natBase)

	githubClient := client.NewGitHubClient(
		httpClient,
		os.Getenv("GITHUB_CLIENT_ID"),
		os.Getenv("GITHUB_CLIENT_SECRET"),
	)

	// ── Step 7: Repositories ──────────────────────────────────────────────────
	profileRepo := repository.NewProfileRepository(pool)
	userRepo := repository.NewUserRepository(pool)

	// ── Step 8: Services ──────────────────────────────────────────────────────
	parserSvc := service.NewParserService()
	cacheSvc := service.NewCacheServiceFromEnv(60 * time.Second)
	profileSvc := service.NewProfileService(profileRepo, genderizeClient, agifyClient, natClient, cacheSvc)
	authSvc := service.NewAuthService(userRepo, githubClient)

	// ── Step 9: Handlers ──────────────────────────────────────────────────────
	profileHdl := handler.NewProfileHandler(profileSvc, parserSvc)
	authHdl := handler.NewAuthHandler(authSvc)

	// ── Step 10: Router ───────────────────────────────────────────────────────
	r := chi.NewRouter()

	// Global middleware (all routes)
	r.Use(middleware.APICors)
	r.Use(middleware.Logger)
	r.Use(chiMiddleware.Recoverer)

	// 404 / 405 handlers
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		response.Error(w, http.StatusNotFound, "route not found")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		response.Error(w, http.StatusNotFound, "route not found")
	})

	// Auth routes (public, rate-limited per IP)
	r.With(middleware.AuthRateLimit).Get("/auth/github", authHdl.RedirectToGitHub)
	r.With(middleware.AuthRateLimit).Get("/auth/github/callback", authHdl.HandleCallback)
	r.With(middleware.AuthRateLimit).Post("/auth/refresh", authHdl.Refresh)
	r.With(middleware.AuthRateLimit).Post("/auth/logout", authHdl.Logout)

	// API routes (protected)
	jwtAuth := middleware.JWTAuth(userRepo)
	r.Group(func(r chi.Router) {
		r.Use(jwtAuth)
		r.Use(middleware.APIVersion)
		r.Use(middleware.APIRateLimit)

		r.Get("/api/users/me", authHdl.Me)

		r.Get("/api/profiles/search", profileHdl.Search)
		r.Get("/api/profiles/export", profileHdl.Export)
		r.Get("/api/profiles", profileHdl.List)
		r.Get("/api/profiles/{id}", profileHdl.Get)

		r.With(middleware.RequireRole("admin")).Post("/api/profiles", profileHdl.Create)
		r.With(middleware.RequireRole("admin")).Post("/api/profiles/import", profileHdl.ImportCSV)
		r.With(middleware.RequireRole("admin")).Delete("/api/profiles/{id}", profileHdl.Delete)
	})

	slog.Info("Step 10: HTTP Server / Router initialized")
	return r
}
