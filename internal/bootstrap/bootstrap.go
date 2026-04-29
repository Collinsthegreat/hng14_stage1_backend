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
	pool, err := pgxpool.New(ctx, dbURL)
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
	profileSvc := service.NewProfileService(profileRepo, genderizeClient, agifyClient, natClient)
	authSvc := service.NewAuthService(userRepo, githubClient)

	// ── Step 9: Handlers ──────────────────────────────────────────────────────
	profileHdl := handler.NewProfileHandler(profileSvc, parserSvc)
	authHdl := handler.NewAuthHandler(authSvc)

	// ── Step 10: Router ───────────────────────────────────────────────────────
	r := chi.NewRouter()

	// Global middleware (outermost first per spec)
	// We only use Logger, Recoverer, StripSlashes globally.
	// CORS is applied per route group to allow wildcard for auth and strict for API.
	r.Use(middleware.Logger)
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.StripSlashes)

	// 404 / 405 handlers
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		response.Error(w, http.StatusNotFound, "route not found")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		response.Error(w, http.StatusNotFound, "route not found")
	})

	// ── Public auth routes (rate-limited per IP) ──────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(middleware.APICors) // Wildcard CORS for auth
		r.Use(middleware.AuthRateLimit)
		r.Get("/auth/github", authHdl.RedirectToGitHub)
		r.Get("/auth/github/callback", authHdl.HandleCallback)
	})

	// ── Token lifecycle — public (no JWT required) ────────────────────────────
	r.Group(func(r chi.Router) {
		r.Use(middleware.APICors) // Wildcard CORS for auth
		r.Use(middleware.CSRF)
		r.Post("/auth/refresh", authHdl.Refresh)
		r.Post("/auth/logout", authHdl.Logout)
	})

	// ── Protected API routes ──────────────────────────────────────────────────
	// Middleware chain: JWTAuth → APIVersion → (route-level) APIRateLimit → RBAC
	jwtAuth := middleware.JWTAuth(userRepo)

	r.Route("/api/profiles", func(r chi.Router) {
		r.Use(middleware.WebCors) // Requires credentials
		r.Use(middleware.APIVersion)
		r.Use(jwtAuth)
		r.Use(middleware.CSRF)
		r.Use(middleware.APIRateLimit)

		// Analyst + Admin (read operations) — order matters: /search and /export before /{id}
		r.Get("/search", profileHdl.Search)
		r.Get("/export", profileHdl.Export)
		r.Get("/", profileHdl.List)
		r.Get("/{id}", profileHdl.Get)

		// Admin only
		r.With(middleware.RequireRole("admin")).Post("/", profileHdl.Create)
		r.With(middleware.RequireRole("admin")).Delete("/{id}", profileHdl.Delete)
	})

	slog.Info("Step 10: HTTP Server / Router initialized")
	return r
}
