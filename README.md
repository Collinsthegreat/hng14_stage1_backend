# Insighta Labs+ Backend

This repository is the Stage 3 backend upgrade for Insighta Labs+, serving a multi-interface platform (Web Portal and CLI).

## System Architecture

The system consists of three main components:
1. **Backend API (this repo):** Handles data storage, external API integration, authentication, and logic.
2. **Web Portal (separate repo):** Server-side rendered or Next.js app interacting via HTTP cookies.
3. **CLI (separate repo):** Interacts with the backend via explicit Bearer tokens using PKCE authentication.

All clients connect to this backend for their operations.

## Authentication Flow

### Browser OAuth Flow
1. **GET `/auth/github`** → Backend generates a random `state`, stores it, and redirects to GitHub.
2. User authenticates on GitHub.
3. **GET `/auth/github/callback`** → GitHub redirects back with `code` and `state`.
4. Backend validates `state` and exchanges `code` for a GitHub access token.
5. Backend fetches GitHub user information and upserts the user in the database.
6. Backend issues an access token (JWT, 3min) and refresh token (opaque, hashed, 5min).
7. Backend sets HTTP-only cookies (`access_token`, `refresh_token`) and redirects to the web portal dashboard.

### CLI PKCE Flow
1. CLI generates a `state`, `code_verifier`, and `code_challenge`.
2. CLI starts a local HTTP server and opens the browser to `/auth/github?code_challenge=...`.
3. Backend redirects to GitHub. After auth, GitHub redirects to `/auth/github/callback`.
4. Backend validates `state` and exchanges `code` and `code_verifier` with GitHub.
5. Backend issues tokens and returns them as a JSON payload to the CLI's callback server.
6. CLI stores the tokens locally (`~/.insighta/credentials.json`) and prints a success message.

## Token Handling

- **Access token:** JWT, 3 minutes expiry, sent as an Authorization header (Bearer) or an HTTP-only cookie.
- **Refresh token:** Opaque token, SHA-256 hashed in DB, 5 minutes expiry. Rotated on every use. Only one valid refresh token per user at a time.
- **CLI auto-refresh:** The CLI automatically intercepts 401 Unauthorized responses, attempts a token refresh via `/auth/refresh`, and transparently retries the original request.

## Role Enforcement

Users are assigned roles upon creation.
- **admin:** Full access (GET, POST, DELETE).
- **analyst:** Read-only (GET only).

All `/api/*` routes go through this middleware chain: `APIVersion → JWTAuth → RBAC`.

## CLI Usage

Install the CLI:
```bash
go install github.com/Collinsthegreat/hng14_stage1_cli/cmd/insighta@latest
```

Available commands:
```bash
insighta login
insighta logout
insighta whoami

# Listing Profiles
insighta profiles list
insighta profiles list --gender male
insighta profiles list --country NG --age-group adult
insighta profiles list --min-age 25 --max-age 40
insighta profiles list --sort-by age --order desc
insighta profiles list --page 2 --limit 20

# Get a single profile
insighta profiles get <id>

# Natural language search
insighta profiles search "young males from nigeria"

# Creating a profile
insighta profiles create --name "Harriet Tubman"

# Exporting profiles to CSV
insighta profiles export --format csv
insighta profiles export --format csv --gender male --country NG
```

## Natural Language Parsing

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

### Limitations
- Only one gender can be matched per query (last match wins)
- Country names must be in the supported lookup map
- Ambiguous queries like "people" with no qualifiers return 422
- No synonym support beyond listed keywords
- Spelling errors are not corrected
- Complex logical operators (OR, NOT) are not supported
- "young" always maps to 16–24 regardless of context

## API Reference

### POST `/api/profiles`
Create a new profile (Admin only).
**Response:** `201 Created`

### GET `/api/profiles`
List profiles with pagination.
**Response:**
```json
{
  "status": "success",
  "page": 1,
  "limit": 10,
  "total": 2026,
  "total_pages": 203,
  "links": {
    "self": "/api/profiles?page=1&limit=10",
    "next": "/api/profiles?page=2&limit=10",
    "prev": null
  },
  "data": [ ... ]
}
```

### GET `/api/profiles/{id}`
Retrieve a profile by ID.

### GET `/api/profiles/search`
Search profiles via NLP. Returns paginated response.

### GET `/api/profiles/export?format=csv`
Export profiles as a CSV file.

### DELETE `/api/profiles/{id}`
Delete a profile (Admin only).

## Live URLs & Repositories

- **Backend Repo:** [https://github.com/Collinsthegreat/hng14_stage1_backend](https://github.com/Collinsthegreat/hng14_stage1_backend)
- **CLI Repo:** [https://github.com/Collinsthegreat/hng14_stage1_cli](https://github.com/Collinsthegreat/hng14_stage1_cli)
- **Web Portal Repo:** [https://github.com/Collinsthegreat/hng14_stage1_web](https://github.com/Collinsthegreat/hng14_stage1_web)

- **Live Backend:** https://hng14-stage1-backend.vercel.app
- **Live Web Portal:** (Vercel deployment URL here once deployed)
- **CLI Install:** `go install github.com/Collinsthegreat/hng14_stage1_cli/cmd/insighta@latest`
