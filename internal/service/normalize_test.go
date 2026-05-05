package service

import (
	"testing"

	"github.com/Collinsthegreat/hng14_stage1_backend/internal/repository"
)

func TestNormalizeFilterCanonicalizesValues(t *testing.T) {
	gender := " Female "
	ageGroup := " Adult "
	countryID := " ng "
	minAge, maxAge := 45, 20
	limit := 500

	filter := NormalizeFilter(repository.ProfileFilter{
		Gender:    &gender,
		AgeGroup:  &ageGroup,
		CountryID: &countryID,
		MinAge:    &minAge,
		MaxAge:    &maxAge,
		Order:     "DESC",
		Page:      0,
		Limit:     limit,
	})

	if *filter.Gender != "female" {
		t.Fatalf("gender = %q, want female", *filter.Gender)
	}
	if *filter.AgeGroup != "adult" {
		t.Fatalf("age_group = %q, want adult", *filter.AgeGroup)
	}
	if *filter.CountryID != "NG" {
		t.Fatalf("country_id = %q, want NG", *filter.CountryID)
	}
	if *filter.MinAge != 20 || *filter.MaxAge != 45 {
		t.Fatalf("age bounds = %d/%d, want 20/45", *filter.MinAge, *filter.MaxAge)
	}
	if filter.SortBy != "created_at" || filter.Order != "desc" {
		t.Fatalf("sort = %s/%s, want created_at/desc", filter.SortBy, filter.Order)
	}
	if filter.Page != 1 || filter.Limit != 50 {
		t.Fatalf("pagination = %d/%d, want 1/50", filter.Page, filter.Limit)
	}
}

func TestEquivalentSearchQueriesProduceSameCacheKey(t *testing.T) {
	parser := NewParserService()

	a, err := parser.ParseSearchQuery("Nigerian females between ages 20 and 45")
	if err != nil {
		t.Fatalf("parse first query: %v", err)
	}
	b, err := parser.ParseSearchQuery("Women aged 20-45 living in Nigeria")
	if err != nil {
		t.Fatalf("parse second query: %v", err)
	}

	if CacheKey(a) != CacheKey(b) {
		t.Fatalf("equivalent female Nigeria queries produced different cache keys: %#v vs %#v", NormalizeFilter(a), NormalizeFilter(b))
	}

	c, err := parser.ParseSearchQuery("young males from nigeria")
	if err != nil {
		t.Fatalf("parse third query: %v", err)
	}
	d, err := parser.ParseSearchQuery("males aged 16 to 24 from NG")
	if err != nil {
		t.Fatalf("parse fourth query: %v", err)
	}

	if CacheKey(c) != CacheKey(d) {
		t.Fatalf("equivalent young male Nigeria queries produced different cache keys: %#v vs %#v", NormalizeFilter(c), NormalizeFilter(d))
	}
}
