# Profiles API

## Tech Stack
- Go 1.21+
- chi router
- PostgreSQL (Vercel Postgres / Neon)
- pgx/v5

## Project Structure
```text
.
├── cmd/
│   └── server/
│       └── main.go
├── internal/
│   ├── bootstrap/
│   │   └── bootstrap.go
│   ├── handler/
│   │   └── profile.go
│   ├── service/
│   │   └── profile.go
│   ├── repository/
│   │   └── profile.go
│   ├── model/
│   │   └── profile.go
│   ├── client/
│   │   ├── genderize.go
│   │   ├── agify.go
│   │   └── nationalize.go
│   └── middleware/
│       └── cors.go
├── pkg/
│   └── response/
│       └── response.go
├── db/
│   └── migrations/
│       └── 001_create_profiles.sql
├── api/
│   └── index.go
├── vercel.json
├── .env.example
├── .gitignore
├── go.mod
└── README.md
```

## Local Setup
```bash
git clone <repo-url>
cd <repo>
cp .env.example .env
# Set DATABASE_URL from Vercel Postgres dashboard
go mod tidy
go run ./cmd/server
```

## Environment Variables
| Variable | Default | Description |
|---|---|---|
| DATABASE_URL | — | Vercel Postgres connection string |
| PORT | 8080 | HTTP port |
| HTTP_TIMEOUT_SECONDS | 5 | External API timeout |

## API Reference

### POST /api/profiles
### GET /api/profiles?gender=&country_id=&age_group=
### GET /api/profiles/{id}
### DELETE /api/profiles/{id}

## Live URL
https://yourapp.vercel.app
