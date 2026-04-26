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
https://hng14-stage1-backend.vercel.app

## Natural Language Search — How It Works

### Supported Keywords & Mappings

| Keyword / Pattern         | Filter Applied                        |
|---------------------------|---------------------------------------|
| male, males, man, men     | gender=male                           |
| female, females, woman    | gender=female                         |
| young, youth              | min_age=16, max_age=24                |
| child, children, kids     | age_group=child                       |
| teenager, teen            | age_group=teenager                    |
| adult, adults             | age_group=adult                       |
| senior, seniors, elderly  | age_group=senior                      |
| above {n}, over {n}       | min_age=n                             |
| below {n}, under {n}      | max_age=n                             |
| between {n} and {m}       | min_age=n, max_age=m                  |
| from {country}            | country_id={ISO code}                 |

### Parsing Logic
1. Lowercase and trim query
2. Detect gender keywords
3. Detect "young/youth" → age range 16–24
4. Detect age group keywords
5. Detect age modifiers (above/below/between)
6. Detect country via "from X" / "in X" pattern + country lookup map
7. All detected filters combined as AND conditions
8. Zero filters extracted → 422 "Unable to interpret query"

### Limitations
- Only one gender can be matched per query (last match wins)
- Country names must be in the supported lookup map
- Ambiguous queries like "people" with no qualifiers return 422
- No synonym support beyond listed keywords
- Spelling errors are not corrected
- Complex logical operators (OR, NOT) are not supported
- "young" always maps to 16–24 regardless of context
