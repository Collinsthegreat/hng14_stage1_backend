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
	"github.com/username/repo-name/internal/client"
	"github.com/username/repo-name/internal/handler"
	"github.com/username/repo-name/internal/middleware"
	"github.com/username/repo-name/internal/repository"
	"github.com/username/repo-name/internal/service"
	"github.com/username/repo-name/pkg/response"
)

func NewRouter() http.Handler {
	// Setup Database Pool
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v", err)
	}

	// Auto-run migrations using embedded SQL
	if migrations.CreateProfilesSQL != "" {
		if _, err := pool.Exec(ctx, migrations.CreateProfilesSQL); err != nil {
			log.Fatalf("Failed to run migrations: %v", err)
		}
	}

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
	svc := service.NewProfileService(repo, genderizeClient, agifyClient, natClient)
	hdl := handler.NewProfileHandler(svc)

	// Setup Router
	r := chi.NewRouter()

	// Global Middleware
	r.Use(middleware.CORS)
	r.Use(middleware.Logger)
	r.Use(chiMiddleware.Recoverer)

	// Custom 404
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		response.Error(w, http.StatusNotFound, "route not found")
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		response.Error(w, http.StatusNotFound, "route not found")
	})

	// Routes
	r.Route("/api/profiles", func(r chi.Router) {
		r.Post("/", hdl.Create)
		r.Get("/", hdl.List)
		r.Get("/{id}", hdl.Get)
		r.Delete("/{id}", hdl.Delete)
	})

	return r
}
