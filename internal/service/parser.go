package service

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/username/repo-name/internal/repository"
)

var countryNameToCode = map[string]string{
	"nigeria":                  "NG",
	"ghana":                    "GH",
	"kenya":                    "KE",
	"angola":                   "AO",
	"ethiopia":                 "ET",
	"tanzania":                 "TZ",
	"uganda":                   "UG",
	"senegal":                  "SN",
	"cameroon":                 "CM",
	"ivory coast":              "CI",
	"côte d'ivoire":            "CI",
	"south africa":             "ZA",
	"egypt":                    "EG",
	"morocco":                  "MA",
	"algeria":                  "DZ",
	"tunisia":                  "TN",
	"benin":                    "BJ",
	"togo":                     "TG",
	"mali":                     "ML",
	"niger":                    "NE",
	"burkina faso":             "BF",
	"somalia":                  "SO",
	"sudan":                    "SD",
	"congo":                    "CG",
	"drc":                      "CD",
	"zambia":                   "ZM",
	"zimbabwe":                 "ZW",
	"mozambique":               "MZ",
	"madagascar":               "MG",
	"rwanda":                   "RW",
	"burundi":                  "BI",
	"malawi":                   "MW",
	"namibia":                  "NA",
	"botswana":                 "BW",
	"lesotho":                  "LS",
	"swaziland":                "SZ",
	"eswatini":                 "SZ",
	"liberia":                  "LR",
	"sierra leone":             "SL",
	"guinea":                   "GN",
	"gambia":                   "GM",
	"mauritania":               "MR",
	"chad":                     "TD",
	"central african republic": "CF",
	"gabon":                    "GA",
	"equatorial guinea":        "GQ",
	"cabo verde":               "CV",
	"united states":            "US",
	"usa":                      "US",
	"uk":                       "GB",
	"united kingdom":           "GB",
	"france":                   "FR",
	"germany":                  "DE",
	"italy":                    "IT",
	"spain":                    "ES",
	"portugal":                 "PT",
	"brazil":                   "BR",
	"india":                    "IN",
	"china":                    "CN",
	"japan":                    "JP",
	"australia":                "AU",
	"canada":                   "CA",
}

type ParserService interface {
	ParseSearchQuery(q string) (repository.ProfileFilter, error)
}

type parserService struct{}

func NewParserService() ParserService {
	return &parserService{}
}

func (s *parserService) ParseSearchQuery(q string) (repository.ProfileFilter, error) {
	lowerQ := strings.TrimSpace(strings.ToLower(q))
	var filter repository.ProfileFilter
	filterExtracted := false

	// Gender keywords
	if strings.Contains(lowerQ, "male and female") || strings.Contains(lowerQ, "both genders") || strings.Contains(lowerQ, "all genders") {
		// no gender filter
	} else if hasWord(lowerQ, []string{"female", "females", "woman", "women", "girl", "girls"}) {
		g := "female"
		filter.Gender = &g
		filterExtracted = true
	} else if hasWord(lowerQ, []string{"male", "males", "man", "men", "boy", "boys"}) {
		g := "male"
		filter.Gender = &g
		filterExtracted = true
	}

	// Young / Youth pattern override
	if strings.Contains(lowerQ, "young") || strings.Contains(lowerQ, "youth") {
		minA, maxA := 16, 24
		filter.MinAge = &minA
		filter.MaxAge = &maxA
		filterExtracted = true
	} else {
		// Age group keywords
		if hasWord(lowerQ, []string{"child", "children", "kids"}) {
			ag := "child"
			filter.AgeGroup = &ag
			filterExtracted = true
		} else if hasWord(lowerQ, []string{"teenager", "teenagers", "teen", "teens"}) {
			ag := "teenager"
			filter.AgeGroup = &ag
			filterExtracted = true
		} else if hasWord(lowerQ, []string{"adult", "adults"}) {
			ag := "adult"
			filter.AgeGroup = &ag
			filterExtracted = true
		} else if hasWord(lowerQ, []string{"senior", "seniors", "elderly"}) {
			ag := "senior"
			filter.AgeGroup = &ag
			filterExtracted = true
		}
	}

	// Age Modifiers
	if minA, ok := extractModifier(lowerQ, []string{"above ", "over ", "older than "}); ok {
		filter.MinAge = &minA
		filterExtracted = true
	}
	if maxA, ok := extractModifier(lowerQ, []string{"below ", "under ", "younger than "}); ok {
		filter.MaxAge = &maxA
		filterExtracted = true
	}
	if minA, maxA, ok := extractBetween(lowerQ); ok {
		filter.MinAge = &minA
		filter.MaxAge = &maxA
		filterExtracted = true
	}

	// Country Modifiers
	for country, code := range countryNameToCode {
		patterns := []string{
			"from " + country,
			"in " + country,
			country + " people",
			country + " citizens",
		}
		for _, pat := range patterns {
			if strings.Contains(lowerQ, pat) {
				c := code
				filter.CountryID = &c
				filterExtracted = true
				break
			}
		}
		if filter.CountryID != nil {
			break
		}
	}

	if !filterExtracted {
		return filter, &ValidationErr{Message: "Unable to interpret query"}
	}

	// Set sort/order/page/limit defaults if they aren't provided by user input usually.
	// We init them safely here, the handler will override Pagination.
	filter.SortBy = "created_at"
	filter.Order = "asc"
	filter.Page = 1
	filter.Limit = 10

	return filter, nil
}

func hasWord(text string, words []string) bool {
	// Pad spaces around to ensure exact match, or use regex
	padded := " " + text + " "
	for _, w := range words {
		if strings.Contains(padded, " "+w+" ") {
			return true
		}
	}
	return false
}

func extractModifier(text string, prefixes []string) (int, bool) {
	for _, p := range prefixes {
		idx := strings.Index(text, p)
		if idx != -1 {
			rem := text[idx+len(p):]
			parts := strings.Fields(rem)
			if len(parts) > 0 {
				numStr := strings.Trim(parts[0], ".,?!")
				if n, err := strconv.Atoi(numStr); err == nil {
					return n, true
				}
			}
		}
	}
	return 0, false
}

func extractBetween(text string) (int, int, bool) {
	re := regexp.MustCompile(`between\s+(\d+)\s+and\s+(\d+)`)
	matches := re.FindStringSubmatch(text)
	if len(matches) == 3 {
		min, err1 := strconv.Atoi(matches[1])
		max, err2 := strconv.Atoi(matches[2])
		if err1 == nil && err2 == nil {
			return min, max, true
		}
	}
	return 0, 0, false
}
