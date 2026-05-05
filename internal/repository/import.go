package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Collinsthegreat/hng14_stage1_backend/internal/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *profileRepository) BatchInsert(ctx context.Context, rows []model.ProfileRow) (inserted, duplicates int, err error) {
	if len(rows) == 0 {
		return 0, 0, nil
	}

	now := time.Now().UTC()
	copyCount, err := r.pool.CopyFrom(
		ctx,
		pgx.Identifier{"profiles"},
		[]string{
			"id",
			"name",
			"gender",
			"gender_probability",
			"sample_size",
			"age",
			"age_group",
			"country_id",
			"country_name",
			"country_probability",
			"created_at",
		},
		pgx.CopyFromSlice(len(rows), func(i int) ([]any, error) {
			id, err := uuid.NewV7()
			if err != nil {
				return nil, err
			}
			return []any{
				id.String(),
				rows[i].Name,
				rows[i].Gender,
				rows[i].GenderProbability,
				0,
				rows[i].Age,
				rows[i].AgeGroup,
				rows[i].CountryID,
				rows[i].CountryName,
				rows[i].CountryProbability,
				now,
			}, nil
		}),
	)
	if err != nil && isDuplicateErr(err) {
		return r.batchInsertWithConflict(ctx, rows)
	}
	return int(copyCount), 0, err
}

func (r *profileRepository) batchInsertWithConflict(ctx context.Context, rows []model.ProfileRow) (inserted, duplicates int, err error) {
	const subBatchSize = 100
	for i := 0; i < len(rows); i += subBatchSize {
		end := i + subBatchSize
		if end > len(rows) {
			end = len(rows)
		}

		ins, dups, e := r.insertSubBatch(ctx, rows[i:end])
		inserted += ins
		duplicates += dups
		if e != nil {
			err = e
		}
	}
	return inserted, duplicates, err
}

func (r *profileRepository) insertSubBatch(ctx context.Context, rows []model.ProfileRow) (inserted, duplicates int, err error) {
	if len(rows) == 0 {
		return 0, 0, nil
	}

	var b strings.Builder
	b.WriteString(`INSERT INTO profiles (
		id, name, gender, gender_probability, sample_size, age, age_group,
		country_id, country_name, country_probability, created_at
	) VALUES `)

	args := make([]any, 0, len(rows)*11)
	now := time.Now().UTC()
	for i, row := range rows {
		if i > 0 {
			b.WriteString(",")
		}
		base := i*11 + 1
		b.WriteString(fmt.Sprintf(
			"($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)",
			base, base+1, base+2, base+3, base+4, base+5,
			base+6, base+7, base+8, base+9, base+10,
		))

		id, err := uuid.NewV7()
		if err != nil {
			return inserted, duplicates, err
		}
		args = append(args,
			id.String(),
			row.Name,
			row.Gender,
			row.GenderProbability,
			0,
			row.Age,
			row.AgeGroup,
			row.CountryID,
			row.CountryName,
			row.CountryProbability,
			now,
		)
	}
	b.WriteString(" ON CONFLICT (name) DO NOTHING")

	tag, err := r.pool.Exec(ctx, b.String(), args...)
	if err != nil {
		return 0, 0, err
	}

	inserted = int(tag.RowsAffected())
	duplicates = len(rows) - inserted
	return inserted, duplicates, nil
}

func isDuplicateErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "duplicate") || strings.Contains(msg, "unique constraint")
}
