package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/username/repo-name/internal/client"
	"github.com/username/repo-name/internal/model"
	"github.com/username/repo-name/internal/repository"
	"golang.org/x/sync/errgroup"
)

type ValidationErr struct {
	Message string
}

func (e *ValidationErr) Error() string { return e.Message }

func IsValidationErr(err error) bool {
	_, ok := err.(*ValidationErr)
	return ok
}

type ProfileService interface {
	CreateProfile(ctx context.Context, req model.CreateProfileRequest) (*model.Profile, bool, error)
	GetProfile(ctx context.Context, id string) (*model.Profile, error)
	ListProfiles(ctx context.Context, f repository.ProfileFilter) ([]model.Profile, int, error)
	DeleteProfile(ctx context.Context, id string) error
}

type profileService struct {
	repo        repository.ProfileRepository
	genderize   client.GenderizeClient
	agify       client.AgifyClient
	nationalize client.NationalizeClient
}

func NewProfileService(repo repository.ProfileRepository, genderize client.GenderizeClient, agify client.AgifyClient, nat client.NationalizeClient) ProfileService {
	return &profileService{repo: repo, genderize: genderize, agify: agify, nationalize: nat}
}

var nameRegex = regexp.MustCompile(`^[a-zA-Z\-]+$`)

func (s *profileService) CreateProfile(ctx context.Context, req model.CreateProfileRequest) (*model.Profile, bool, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, false, &ValidationErr{Message: "name is required"}
	}
	if !nameRegex.MatchString(name) {
		return nil, false, &ValidationErr{Message: "name must be a valid string"}
	}
	name = strings.ToLower(name)

	existingProfile, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return nil, false, fmt.Errorf("db error: %w", err)
	}
	if existingProfile != nil {
		return existingProfile, true, nil
	}

	g, gctx := errgroup.WithContext(ctx)

	var genderRes client.GenderizeResponse
	var agifyRes client.AgifyResponse
	var natRes client.NationalizeResponse

	g.Go(func() error {
		r, err := s.genderize.Fetch(gctx, name)
		if err != nil {
			return err
		}
		genderRes = r
		return nil
	})
	g.Go(func() error {
		r, err := s.agify.Fetch(gctx, name)
		if err != nil {
			return err
		}
		agifyRes = r
		return nil
	})
	g.Go(func() error {
		r, err := s.nationalize.Fetch(gctx, name)
		if err != nil {
			return err
		}
		natRes = r
		return nil
	})

	if err := g.Wait(); err != nil {
		return nil, false, err // Handled cleanly as 502 with message by caller
	}

	id, err := uuid.NewV7()
	if err != nil {
		return nil, false, fmt.Errorf("uuid error: %w", err)
	}

	bestCountry := topCountry(natRes.Countries)

	p := &model.Profile{
		ID:                 id.String(),
		Name:               name,
		Gender:             *genderRes.Gender,
		GenderProbability:  genderRes.Probability,
		SampleSize:         genderRes.Count,
		Age:                *agifyRes.Age,
		AgeGroup:           classifyAgeGroup(*agifyRes.Age),
		CountryID:          bestCountry.CountryID,
		CountryName:        "Unknown", // A simple default for new Stage1 API posts since Nationalize only returns ID
		CountryProbability: bestCountry.Probability,
		CreatedAt:          time.Now().UTC(),
	}

	if err := s.repo.Create(ctx, p); err != nil {
		return nil, false, fmt.Errorf("db error: %w", err)
	}

	return p, false, nil
}

func (s *profileService) GetProfile(ctx context.Context, id string) (*model.Profile, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *profileService) ListProfiles(ctx context.Context, f repository.ProfileFilter) ([]model.Profile, int, error) {
	return s.repo.List(ctx, f)
}

func (s *profileService) DeleteProfile(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func classifyAgeGroup(age int) string {
	switch {
	case age <= 12:
		return "child"
	case age <= 19:
		return "teenager"
	case age <= 59:
		return "adult"
	default:
		return "senior"
	}
}

func topCountry(countries []client.CountryInfo) client.CountryInfo {
	best := countries[0]
	for _, c := range countries[1:] {
		if c.Probability > best.Probability {
			best = c
		}
	}
	return best
}
