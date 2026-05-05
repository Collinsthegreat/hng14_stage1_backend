package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/Collinsthegreat/hng14_stage1_backend/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ProfileFilter struct {
	Gender         *string
	AgeGroup       *string
	CountryID      *string
	MinAge         *int
	MaxAge         *int
	MinGenderProb  *float64
	MinCountryProb *float64
	SortBy         string
	Order          string
	Page           int
	Limit          int
}

type ProfileRepository interface {
	Create(ctx context.Context, p *model.Profile) error
	GetByID(ctx context.Context, id string) (*model.Profile, error)
	GetByName(ctx context.Context, name string) (*model.Profile, error)
	List(ctx context.Context, f ProfileFilter) ([]model.Profile, int, error)
	ListAll(ctx context.Context, f ProfileFilter) ([]model.Profile, error)
	BatchInsert(ctx context.Context, rows []model.ProfileRow) (inserted, duplicates int, err error)
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
			age, age_group, country_id, country_name, country_probability, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		)
	`
	_, err := r.pool.Exec(ctx, query,
		p.ID, p.Name, p.Gender, p.GenderProbability, p.SampleSize,
		p.Age, p.AgeGroup, p.CountryID, p.CountryName, p.CountryProbability, p.CreatedAt,
	)
	return err
}

func (r *profileRepository) GetByID(ctx context.Context, id string) (*model.Profile, error) {
	query := `
		SELECT id, name, gender, gender_probability, sample_size, 
			   age, age_group, country_id, country_name, country_probability, created_at
		FROM profiles
		WHERE id = $1
	`
	var p model.Profile
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&p.ID, &p.Name, &p.Gender, &p.GenderProbability, &p.SampleSize,
		&p.Age, &p.AgeGroup, &p.CountryID, &p.CountryName, &p.CountryProbability, &p.CreatedAt,
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
			   age, age_group, country_id, country_name, country_probability, created_at
		FROM profiles
		WHERE LOWER(name) = LOWER($1)
	`
	var p model.Profile
	err := r.pool.QueryRow(ctx, query, name).Scan(
		&p.ID, &p.Name, &p.Gender, &p.GenderProbability, &p.SampleSize,
		&p.Age, &p.AgeGroup, &p.CountryID, &p.CountryName, &p.CountryProbability, &p.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func buildProfileWhereClause(f ProfileFilter) (string, []any) {
	args := []any{}
	argN := 1
	where := []string{}

	if f.Gender != nil {
		where = append(where, fmt.Sprintf("LOWER(gender) = $%d", argN))
		args = append(args, strings.ToLower(*f.Gender))
		argN++
	}
	if f.AgeGroup != nil {
		where = append(where, fmt.Sprintf("LOWER(age_group) = $%d", argN))
		args = append(args, strings.ToLower(*f.AgeGroup))
		argN++
	}
	if f.CountryID != nil {
		where = append(where, fmt.Sprintf("LOWER(country_id) = $%d", argN))
		args = append(args, strings.ToLower(*f.CountryID))
		argN++
	}
	if f.MinAge != nil {
		where = append(where, fmt.Sprintf("age >= $%d", argN))
		args = append(args, *f.MinAge)
		argN++
	}
	if f.MaxAge != nil {
		where = append(where, fmt.Sprintf("age <= $%d", argN))
		args = append(args, *f.MaxAge)
		argN++
	}
	if f.MinGenderProb != nil {
		where = append(where, fmt.Sprintf("gender_probability >= $%d", argN))
		args = append(args, *f.MinGenderProb)
		argN++
	}
	if f.MinCountryProb != nil {
		where = append(where, fmt.Sprintf("country_probability >= $%d", argN))
		args = append(args, *f.MinCountryProb)
	}

	if len(where) == 0 {
		return "", args
	}
	return "WHERE " + strings.Join(where, " AND "), args
}

func profileSort(f ProfileFilter) (string, string) {
	allowedSortBy := map[string]string{
		"age":                "age",
		"created_at":         "created_at",
		"gender_probability": "gender_probability",
	}
	sortCol, ok := allowedSortBy[f.SortBy]
	if !ok {
		sortCol = "created_at"
	}

	orderDir := "ASC"
	if strings.ToLower(f.Order) == "desc" {
		orderDir = "DESC"
	}
	return sortCol, orderDir
}

func (r *profileRepository) List(ctx context.Context, f ProfileFilter) ([]model.Profile, int, error) {
	var total int
	var profiles []model.Profile
	var countErr, dataErr error

	whereClause, args := buildProfileWhereClause(f)
	sortCol, orderDir := profileSort(f)
	limitArg := len(args) + 1
	offsetArg := len(args) + 2
	offset := (f.Page - 1) * f.Limit

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		countQuery := fmt.Sprintf("SELECT COUNT(*) FROM profiles %s", whereClause)
		countErr = r.pool.QueryRow(ctx, countQuery, args...).Scan(&total)
	}()

	go func() {
		defer wg.Done()
		dataQuery := fmt.Sprintf(
			`SELECT id,name,gender,gender_probability,age,age_group,country_id,country_name,country_probability,created_at
			 FROM profiles %s ORDER BY %s %s LIMIT $%d OFFSET $%d`,
			whereClause, sortCol, orderDir, limitArg, offsetArg,
		)
		dataArgs := append(append([]any(nil), args...), f.Limit, offset)
		rows, err := r.pool.Query(ctx, dataQuery, dataArgs...)
		if err != nil {
			dataErr = fmt.Errorf("data query error: %w", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var p model.Profile
			if err := rows.Scan(
				&p.ID, &p.Name, &p.Gender, &p.GenderProbability,
				&p.Age, &p.AgeGroup, &p.CountryID, &p.CountryName,
				&p.CountryProbability, &p.CreatedAt,
			); err != nil {
				dataErr = fmt.Errorf("row scan error: %w", err)
				return
			}
			profiles = append(profiles, p)
		}
		if err := rows.Err(); err != nil {
			dataErr = err
		}
	}()

	wg.Wait()
	if countErr != nil {
		return nil, 0, fmt.Errorf("count query error: %w", countErr)
	}
	if dataErr != nil {
		return nil, 0, dataErr
	}

	if profiles == nil {
		profiles = make([]model.Profile, 0)
	}

	return profiles, total, nil
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

// ListAll returns all matching profiles without pagination.
// A hard server-side cap of 10,000 rows is enforced to prevent memory exhaustion.
const listAllRowCap = 10_000

func (r *profileRepository) ListAll(ctx context.Context, f ProfileFilter) ([]model.Profile, error) {
	whereClause, args := buildProfileWhereClause(f)
	sortCol, orderDir := profileSort(f)

	dataQuery := fmt.Sprintf(
		`SELECT id,name,gender,gender_probability,age,age_group,country_id,country_name,country_probability,created_at
		 FROM profiles %s ORDER BY %s %s LIMIT $%d`,
		whereClause, sortCol, orderDir, len(args)+1,
	)
	args = append(args, listAllRowCap)

	rows, err := r.pool.Query(ctx, dataQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("listAll query error: %w", err)
	}
	defer rows.Close()

	var profiles []model.Profile
	for rows.Next() {
		var p model.Profile
		if err := rows.Scan(
			&p.ID, &p.Name, &p.Gender, &p.GenderProbability,
			&p.Age, &p.AgeGroup, &p.CountryID, &p.CountryName,
			&p.CountryProbability, &p.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("listAll row scan error: %w", err)
		}
		profiles = append(profiles, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if profiles == nil {
		profiles = make([]model.Profile, 0)
	}
	return profiles, nil
}
