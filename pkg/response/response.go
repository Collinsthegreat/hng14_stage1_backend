package response

import (
	"encoding/json"
	"math"
	"net/http"
	"net/url"
	"strconv"
)

// ─── Response types ────────────────────────────────────────────────────────────

type ErrorResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

type SuccessResponse struct {
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	Data    any    `json:"data"`
}

type ListResponse struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
	Data   any    `json:"data"`
}

// PaginatedLinks holds the hypermedia navigation links for a paginated response.
type PaginatedLinks struct {
	Self *string `json:"self"`
	Next *string `json:"next"`
	Prev *string `json:"prev"`
}

// PaginatedResponse is the canonical shape for all list/search endpoints.
type PaginatedResponse struct {
	Status     string         `json:"status"`
	Page       int            `json:"page"`
	Limit      int            `json:"limit"`
	Total      int            `json:"total"`
	TotalPages int            `json:"total_pages"`
	Links      PaginatedLinks `json:"links"`
	Data       any            `json:"data"`
}

// ─── Writers ──────────────────────────────────────────────────────────────────

func JSON(w http.ResponseWriter, statusCode int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(data)
}

func Success(w http.ResponseWriter, statusCode int, data any) {
	JSON(w, statusCode, SuccessResponse{
		Status: "success",
		Data:   data,
	})
}

func SuccessWithMessage(w http.ResponseWriter, statusCode int, message string, data any) {
	JSON(w, statusCode, SuccessResponse{
		Status:  "success",
		Message: message,
		Data:    data,
	})
}

func List(w http.ResponseWriter, count int, data any) {
	JSON(w, http.StatusOK, ListResponse{
		Status: "success",
		Count:  count,
		Data:   data,
	})
}

// PaginatedList builds the paginated response with total_pages and navigation links.
// basePath is the route path (e.g. "/api/profiles"); queryParams preserves all active filters.
func PaginatedList(w http.ResponseWriter, page, limit, total int, basePath string, queryParams url.Values, data any) {
	totalPages := int(math.Ceil(float64(total) / float64(limit)))
	if totalPages < 1 {
		totalPages = 1
	}

	makeLink := func(p int) *string {
		q := make(url.Values)
		// Copy all existing filter params
		for k, vals := range queryParams {
			for _, v := range vals {
				q.Set(k, v)
			}
		}
		q.Set("page", strconv.Itoa(p))
		q.Set("limit", strconv.Itoa(limit))
		s := basePath + "?" + q.Encode()
		return &s
	}

	var nextLink, prevLink *string
	if page < totalPages {
		nextLink = makeLink(page + 1)
	}
	if page > 1 {
		prevLink = makeLink(page - 1)
	}

	JSON(w, http.StatusOK, PaginatedResponse{
		Status:     "success",
		Page:       page,
		Limit:      limit,
		Total:      total,
		TotalPages: totalPages,
		Links: PaginatedLinks{
			Self: makeLink(page),
			Next: nextLink,
			Prev: prevLink,
		},
		Data: data,
	})
}

func Error(w http.ResponseWriter, statusCode int, message string) {
	JSON(w, statusCode, ErrorResponse{
		Status:  "error",
		Message: message,
	})
}
