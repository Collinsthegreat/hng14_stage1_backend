package bootstrap

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/username/repo-name/db/migrations"
	"github.com/username/repo-name/db/seed"
	"github.com/username/repo-name/internal/client"
	"github.com/username/repo-name/internal/handler"
	"github.com/username/repo-name/internal/middleware"
	"github.com/username/repo-name/internal/repository"
	"github.com/username/repo-name/internal/service"
	"github.com/username/repo-name/pkg/response"
)

func NewRouter() http.Handler {
	// Startup Sequence Step 1: Connect to DB pool
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v", err)
	}

	log.Println("Step 1: DB Pool connected")

	// Startup Sequence Step 2: Run migration 001
	if migrations.CreateProfilesSQL != "" {
		if _, err := pool.Exec(ctx, migrations.CreateProfilesSQL); err != nil {
			log.Fatalf("Failed to run migration 001: %v", err)
		}
	}
	log.Println("Step 2: Migration 001 completed")

	// Startup Sequence Step 3: Run migration 002
	if migrations.AddCountryNameSQL != "" {
		if _, err := pool.Exec(ctx, migrations.AddCountryNameSQL); err != nil {
			log.Fatalf("Failed to run migration 002: %v", err)
		}
	}
	log.Println("Step 3: Migration 002 completed")

	// Startup Sequence Step 4: Run Seeding
	if err := seed.SeedProfiles(ctx, pool); err != nil {
		log.Fatalf("Failed to run seed profiles: %v", err)
	}
	log.Println("Step 4: SeedProfiles completed")

	// Setup external clients
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

	// Setup Layers
	repo := repository.NewProfileRepository(pool)
	// Create NLP parser service
	parserSvc := service.NewParserService()
	svc := service.NewProfileService(repo, genderizeClient, agifyClient, natClient)
	hdl := handler.NewProfileHandler(svc, parserSvc)

	// Setup Router
	r := chi.NewRouter()

	// Global Middleware
	r.Use(middleware.CORS)
	r.Use(middleware.Logger)
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.StripSlashes)

	// Custom 404
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		response.Error(w, http.StatusNotFound, "route not found")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		response.Error(w, http.StatusNotFound, "route not found")
	})

	// Routes
	// ORDER-SENSITIVE: /search MUST be registered before /{id}
	r.Get("/api/profiles/search", hdl.Search)
	r.Post("/api/profiles", hdl.Create)
	r.Get("/api/profiles", hdl.List)
	r.Get("/api/profiles/{id}", hdl.Get)
	r.Delete("/api/profiles/{id}", hdl.Delete)

	log.Println("Step 5: HTTP Server / Router initialized")
	return r
}
