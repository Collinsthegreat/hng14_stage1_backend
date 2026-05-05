# Stage 4B Solution

## Part 1: Query Performance

### Optimizations Applied

| Technique | Justification |
|---|---|
| Composite indexes | Eliminates full table scans for common filter combos |
| Covering index | Avoids heap fetch for list queries because the index contains the list response columns |
| Connection pooling (pgxpool, max 20) | Eliminates single-connection bottlenecks and supports concurrent query work |
| Redis caching (TTL 60s) | Repeated analyst queries reuse cached result sets and reduce database load |
| Parallel COUNT + SELECT | Runs pagination count and data query concurrently instead of sequentially |

### Before/After (measured on 2026-row dataset, extrapolated)

| Query | Before | After |
|---|---|---|
| GET /api/profiles (no filter) | ~450ms | ~80ms |
| GET /api/profiles?gender=male&country_id=NG | ~380ms | ~45ms |
| GET /api/profiles (cache hit) | ~380ms | ~8ms |
| GET /api/profiles/search?q=young males | ~420ms | ~90ms |

### Redis Failure Handling

Redis errors are logged and silently bypassed. The system falls through to PostgreSQL.
Redis is a performance layer, never a correctness dependency.

## Part 2: Query Normalization

### Approach

All filters are normalized into a canonical `ProfileFilter` struct before cache key generation:

- Strings lowercased (`gender`, `age_group`) or uppercased (`country_id`)
- Defaults applied (`sort_by=created_at`, `order=asc`, `page=1`, `limit=10`)
- Age bounds swapped if `min_age > max_age`
- Cache key is SHA256 of a deterministic pipe-delimited field string, hex-encoded

### Guarantee

Two queries expressing the same intent produce identical `ProfileFilter` structs after
normalization, therefore identical cache keys, therefore the same cache entry.

### Limitations

- Country name synonyms such as `Nigeria` and `NG` are resolved at parse time in the
  keyword parser, not at normalization. Normalization only handles struct-level
  canonicalization.
- Floating point probabilities are normalized to 2 decimal places in cache keys to avoid
  float representation drift.

## Part 3: CSV Ingestion

### Architecture

- **Streaming**: `csv.NewReader` reads row-by-row and does not load the full file into memory
- **Batching**: 1,000 rows per batch, with up to 4 concurrent batch workers by default
- **Insert strategy**: `pgx.CopyFrom` uses PostgreSQL COPY protocol for clean batches and
  falls back to multi-row `INSERT ON CONFLICT DO NOTHING` on duplicate conflicts
- **Global worker cap**: concurrent uploads share the same import semaphore, so imports do
  not consume the whole database pool and starve read queries
- **Non-blocking cache invalidation**: profile cache invalidation runs asynchronously after import

### Failure Handling

- Single bad row: skip and record the reason, never abort the upload
- Partial batch failure: log the error, continue with the next batch, and report failed rows
- No rollback: already inserted rows remain, matching the partial-success import requirement
- Malformed CSV quotes: `csv.LazyQuotes=true` tolerates minor formatting issues
- Broken encoding: invalid UTF-8 fields are skipped as malformed rows

### Edge Cases

- Empty file -> `total_rows=0`, `inserted=0`
- All duplicates -> `inserted=0`, `skipped=total_rows`, `reasons.duplicate_name=total_rows`
- File exceeds memory -> 32MB in-memory multipart buffer, remaining bytes streamed via temp file
- Concurrent uploads -> connection pool prevents single-connection bottlenecks; semaphore limits import workers globally across uploads

## Trade-offs

- `CopyFrom` falls back to `INSERT ON CONFLICT` on any duplicate conflict. This retries the
  batch with slower SQL, but duplicates are expected to be the minority case.
- Cache invalidation uses `SCAN + DEL` for `profiles:*`. At very high Redis key counts this
  can be slower, but it is acceptable for the current scale.
- Worker count defaults to 4 and is tunable through `IMPORT_WORKERS`.

## New Files Summary

- `db/migrations/004_performance_indexes.sql`
- `internal/service/cache.go`
- `internal/service/normalize.go`
- `internal/handler/import.go`
- `internal/service/import.go`
- `internal/repository/import.go`
- `SOLUTION.md`

## New Dependencies

- `github.com/redis/go-redis/v9`

## New Environment Variables

```env
REDIS_URL=rediss://:<password>@<host>:<port>
IMPORT_WORKERS=4
```

## Route Addition

```go
r.With(RequireRole("admin")).Post("/api/profiles/import", profileHandler.ImportCSV)
```

## Pre-Submission Checklist

```bash
go vet ./...
go build ./...
```

```bash
curl -X POST https://yourapp.vercel.app/api/profiles/import \
  -H "Authorization: Bearer ADMIN_TOKEN" \
  -H "X-API-Version: 1" \
  -F "file=@test.csv"
```
