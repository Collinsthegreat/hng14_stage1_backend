package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/username/repo-name/internal/model"
)

type ProfileRepository interface {
	Create(ctx context.Context, p *model.Profile) error
	GetByID(ctx context.Context, id string) (*model.Profile, error)
	GetByName(ctx context.Context, name string) (*model.Profile, error)
	List(ctx context.Context, gender, countryID, ageGroup string) ([]model.ProfileListItem, error)
	Delete(ctx context.Context, id string) error
}

type profileRepository struct {
	pool *pgxpool.Pool
}

func NewProfileRepository(pool *pgxpool.Pool) ProfileRepository {
	return &profileRepository{pool: pool}
}

func (r *profileRepository) Create(ctx context.Context, p *model.Profile) error {
	query := `
		INSERT INTO profiles (
			id, name, gender, gender_probability, sample_size, 
			age, age_group, country_id, country_probability, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		)
	`
	_, err := r.pool.Exec(ctx, query,
		p.ID, p.Name, p.Gender, p.GenderProbability, p.SampleSize,
		p.Age, p.AgeGroup, p.CountryID, p.CountryProbability, p.CreatedAt,
	)
	return err
}

func (r *profileRepository) GetByID(ctx context.Context, id string) (*model.Profile, error) {
	query := `
		SELECT id, name, gender, gender_probability, sample_size, 
			   age, age_group, country_id, country_probability, created_at
		FROM profiles
		WHERE id = $1
	`
	var p model.Profile
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&p.ID, &p.Name, &p.Gender, &p.GenderProbability, &p.SampleSize,
		&p.Age, &p.AgeGroup, &p.CountryID, &p.CountryProbability, &p.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil // Return nil, nil when not found to differentiate from db error
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *profileRepository) GetByName(ctx context.Context, name string) (*model.Profile, error) {
	query := `
		SELECT id, name, gender, gender_probability, sample_size, 
			   age, age_group, country_id, country_probability, created_at
		FROM profiles
		WHERE LOWER(name) = LOWER($1)
	`
	var p model.Profile
	err := r.pool.QueryRow(ctx, query, name).Scan(
		&p.ID, &p.Name, &p.Gender, &p.GenderProbability, &p.SampleSize,
		&p.Age, &p.AgeGroup, &p.CountryID, &p.CountryProbability, &p.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *profileRepository) List(ctx context.Context, gender, countryID, ageGroup string) ([]model.ProfileListItem, error) {
	query := `
		SELECT id, name, gender, age, age_group, country_id
		FROM profiles
		WHERE 1=1
	`
	var args []any
	argID := 1

	if gender != "" {
		query += fmt.Sprintf(" AND LOWER(gender) = LOWER($%d)", argID)
		args = append(args, gender)
		argID++
	}
	if countryID != "" {
		query += fmt.Sprintf(" AND LOWER(country_id) = LOWER($%d)", argID)
		args = append(args, countryID)
		argID++
	}
	if ageGroup != "" {
		query += fmt.Sprintf(" AND LOWER(age_group) = LOWER($%d)", argID)
		args = append(args, ageGroup)
		argID++
	}

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var profiles []model.ProfileListItem
	for rows.Next() {
		var p model.ProfileListItem
		if err := rows.Scan(&p.ID, &p.Name, &p.Gender, &p.Age, &p.AgeGroup, &p.CountryID); err != nil {
			return nil, err
		}
		profiles = append(profiles, p)
	}
	
	if err := rows.Err(); err != nil {
		return nil, err
	}
	
	if profiles == nil {
		profiles = make([]model.ProfileListItem, 0)
	}

	return profiles, nil
}

func (r *profileRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM profiles WHERE id = $1`
	cmdTag, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("not found")
	}
	return nil
}
