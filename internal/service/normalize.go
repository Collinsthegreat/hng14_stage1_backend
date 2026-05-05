package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/Collinsthegreat/hng14_stage1_backend/internal/repository"
)

func NormalizeFilter(f repository.ProfileFilter) repository.ProfileFilter {
	if f.Gender != nil {
		s := strings.ToLower(strings.TrimSpace(*f.Gender))
		f.Gender = &s
	}
	if f.AgeGroup != nil {
		s := strings.ToLower(strings.TrimSpace(*f.AgeGroup))
		f.AgeGroup = &s
	}
	if f.CountryID != nil {
		s := strings.ToUpper(strings.TrimSpace(*f.CountryID))
		f.CountryID = &s
	}

	if f.SortBy == "" {
		f.SortBy = "created_at"
	}
	if f.Order == "" {
		f.Order = "asc"
	}
	f.Order = strings.ToLower(strings.TrimSpace(f.Order))

	if f.Page < 1 {
		f.Page = 1
	}
	if f.Limit < 1 {
		f.Limit = 10
	}
	if f.Limit > 50 {
		f.Limit = 50
	}

	if f.MinAge != nil && f.MaxAge != nil && *f.MinAge > *f.MaxAge {
		f.MinAge, f.MaxAge = f.MaxAge, f.MinAge
	}

	return f
}

func CacheKey(f repository.ProfileFilter) string {
	f = NormalizeFilter(f)
	parts := []string{"profiles"}

	if f.Gender != nil {
		parts = append(parts, "g:"+*f.Gender)
	}
	if f.AgeGroup != nil {
		parts = append(parts, "ag:"+*f.AgeGroup)
	}
	if f.CountryID != nil {
		parts = append(parts, "c:"+*f.CountryID)
	}
	if f.MinAge != nil {
		parts = append(parts, fmt.Sprintf("mna:%d", *f.MinAge))
	}
	if f.MaxAge != nil {
		parts = append(parts, fmt.Sprintf("mxa:%d", *f.MaxAge))
	}
	if f.MinGenderProb != nil {
		parts = append(parts, fmt.Sprintf("mgp:%.2f", *f.MinGenderProb))
	}
	if f.MinCountryProb != nil {
		parts = append(parts, fmt.Sprintf("mcp:%.2f", *f.MinCountryProb))
	}

	parts = append(parts,
		"sb:"+f.SortBy,
		"o:"+f.Order,
		fmt.Sprintf("p:%d", f.Page),
		fmt.Sprintf("l:%d", f.Limit),
	)

	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return "profiles:" + hex.EncodeToString(sum[:16])
}
