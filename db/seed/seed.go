package seed

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed profiles.json
var profilesJSON []byte

type SeedProfile struct {
	Name               string  `json:"name"`
	Gender             string  `json:"gender"`
	GenderProbability  float64 `json:"gender_probability"`
	Age                int     `json:"age"`
	AgeGroup           string  `json:"age_group"`
	CountryID          string  `json:"country_id"`
	CountryName        string  `json:"country_name"`
	CountryProbability float64 `json:"country_probability"`
}

type profilesData struct {
	Profiles []SeedProfile `json:"profiles"`
}

func SeedProfiles(ctx context.Context, pool *pgxpool.Pool) error {
	var count int
	err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM profiles").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check profiles count: %w", err)
	}

	if count >= 2026 {
		slog.Info("profiles table already seeded", "count", count)
		return nil
	}

	slog.Info("seeding profiles table from profiles.json...")

	var data profilesData
	if err := json.Unmarshal(profilesJSON, &data); err != nil {
		return fmt.Errorf("failed to parse profiles.json: %w", err)
	}

	inserted := 0
	skipped := 0

	now := time.Now().UTC()

	for _, p := range data.Profiles {
		id, err := uuid.NewV7()
		if err != nil {
			return fmt.Errorf("failed to generate uuid: %w", err)
		}

		// Insert with ON CONFLICT DO NOTHING
		sql := `
			INSERT INTO profiles (id, name, gender, gender_probability, age, age_group, country_id, country_name, country_probability, created_at, sample_size)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
			ON CONFLICT (name) DO NOTHING
		`

		tag, err := pool.Exec(ctx, sql,
			id.String(),
			p.Name,
			p.Gender,
			p.GenderProbability,
			p.Age,
			p.AgeGroup,
			p.CountryID,
			p.CountryName,
			p.CountryProbability,
			now,
			// Using 0 as a default for sample_size since we don't have it in JSON but must keep it
			0,
		)
		if err != nil {
			return fmt.Errorf("failed to insert profile %s: %w", p.Name, err)
		}

		if tag.RowsAffected() > 0 {
			inserted++
		} else {
			skipped++
		}
	}

	slog.Info("seeding complete", "inserted", inserted, "skipped", skipped)
	return nil
}
