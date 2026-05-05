package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"unicode/utf8"

	"github.com/Collinsthegreat/hng14_stage1_backend/internal/model"
)

var validGenders = map[string]bool{"male": true, "female": true}

var validAgeGroups = map[string]bool{
	"child":    true,
	"teenager": true,
	"adult":    true,
	"senior":   true,
}

type ImportStats struct {
	TotalRows int64
	Inserted  int64
	Skipped   int64

	mu      sync.Mutex
	Reasons map[string]int64
}

type ImportResponse struct {
	Status    string           `json:"status"`
	TotalRows int64            `json:"total_rows"`
	Inserted  int64            `json:"inserted"`
	Skipped   int64            `json:"skipped"`
	Reasons   map[string]int64 `json:"reasons"`
}

func NewImportStats() *ImportStats {
	return &ImportStats{Reasons: make(map[string]int64)}
}

func (s *ImportStats) AddReason(reason string) {
	s.AddReasonN(reason, 1)
}

func (s *ImportStats) AddReasonN(reason string, n int) {
	if reason == "" || n <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Reasons == nil {
		s.Reasons = make(map[string]int64)
	}
	s.Reasons[reason] += int64(n)
}

func (s *ImportStats) ToResponse() ImportResponse {
	reasons := make(map[string]int64)
	s.mu.Lock()
	for k, v := range s.Reasons {
		reasons[k] = v
	}
	s.mu.Unlock()

	return ImportResponse{
		Status:    "success",
		TotalRows: atomic.LoadInt64(&s.TotalRows),
		Inserted:  atomic.LoadInt64(&s.Inserted),
		Skipped:   atomic.LoadInt64(&s.Skipped),
		Reasons:   reasons,
	}
}

func ParseProfileImportHeader(header []string) (map[string]int, error) {
	required := []string{
		"name",
		"gender",
		"gender_probability",
		"age",
		"age_group",
		"country_id",
		"country_name",
		"country_probability",
	}

	index := make(map[string]int, len(header))
	for i, col := range header {
		index[strings.ToLower(strings.TrimSpace(col))] = i
	}
	for _, col := range required {
		if _, ok := index[col]; !ok {
			return nil, fmt.Errorf("missing required column: %s", col)
		}
	}
	return index, nil
}

func ValidateProfileImportRow(raw []string, colIndex map[string]int) (model.ProfileRow, string, error) {
	for _, field := range raw {
		if !utf8.ValidString(field) {
			return model.ProfileRow{}, "malformed_csv", errors.New("invalid encoding")
		}
	}

	get := func(field string) string {
		i, ok := colIndex[field]
		if !ok || i >= len(raw) {
			return ""
		}
		return strings.TrimSpace(raw[i])
	}

	name := strings.ToLower(get("name"))
	if name == "" {
		return model.ProfileRow{}, "missing_fields", errors.New("missing name")
	}

	gender := strings.ToLower(get("gender"))
	if !validGenders[gender] {
		return model.ProfileRow{}, "invalid_gender", errors.New("invalid gender")
	}

	age, err := strconv.Atoi(get("age"))
	if err != nil || age < 0 || age > 150 {
		return model.ProfileRow{}, "invalid_age", errors.New("invalid age")
	}

	ageGroup := strings.ToLower(get("age_group"))
	if !validAgeGroups[ageGroup] {
		ageGroup = classifyAgeGroup(age)
	}

	countryID := strings.ToUpper(get("country_id"))
	if len(countryID) != 2 {
		return model.ProfileRow{}, "missing_fields", errors.New("invalid country_id")
	}

	countryName := get("country_name")
	if countryName == "" {
		return model.ProfileRow{}, "missing_fields", errors.New("missing country_name")
	}

	genderProb, err := strconv.ParseFloat(get("gender_probability"), 64)
	if err != nil || genderProb < 0 || genderProb > 1 {
		return model.ProfileRow{}, "missing_fields", errors.New("invalid gender_probability")
	}

	countryProb, err := strconv.ParseFloat(get("country_probability"), 64)
	if err != nil || countryProb < 0 || countryProb > 1 {
		return model.ProfileRow{}, "missing_fields", errors.New("invalid country_probability")
	}

	return model.ProfileRow{
		Name:               name,
		Gender:             gender,
		GenderProbability:  genderProb,
		Age:                age,
		AgeGroup:           ageGroup,
		CountryID:          countryID,
		CountryName:        countryName,
		CountryProbability: countryProb,
	}, "", nil
}
