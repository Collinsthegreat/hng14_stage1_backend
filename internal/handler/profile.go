package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/username/repo-name/internal/model"
	"github.com/username/repo-name/internal/repository"
	"github.com/username/repo-name/internal/service"
	"github.com/username/repo-name/pkg/response"
)

type ProfileHandler struct {
	svc    service.ProfileService
	parser service.ParserService
}

func NewProfileHandler(svc service.ProfileService, parser service.ParserService) *ProfileHandler {
	return &ProfileHandler{svc: svc, parser: parser}
}

func (h *ProfileHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		response.Error(w, http.StatusBadRequest, "name is required") // Treat malformed or missing body as 400
		return
	}

	p, exists, err := h.svc.CreateProfile(r.Context(), req)
	if err != nil {
		if service.IsValidationErr(err) {
			if err.Error() == "name is required" {
				response.Error(w, http.StatusBadRequest, err.Error())
				return
			}
			response.Error(w, http.StatusUnprocessableEntity, err.Error())
			return
		}

		// Map explicit 502 messages from clients
		errMsg := err.Error()
		if errMsg == "Genderize returned an invalid response" ||
			errMsg == "Agify returned an invalid response" ||
			errMsg == "Nationalize returned an invalid response" {
			response.Error(w, http.StatusBadGateway, errMsg)
			return
		}

		slog.Error("CreateProfile internal error", "error", err)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}

	if exists {
		response.SuccessWithMessage(w, http.StatusOK, "Profile already exists", p)
		return
	}

	response.Success(w, http.StatusCreated, p)
}

func (h *ProfileHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		response.Error(w, http.StatusNotFound, "Profile not found")
		return
	}

	p, err := h.svc.GetProfile(r.Context(), id)
	if err != nil {
		slog.Error("GetProfile error", "error", err, "id", id)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}

	if p == nil {
		response.Error(w, http.StatusNotFound, "Profile not found")
		return
	}

	response.Success(w, http.StatusOK, p)
}

func (h *ProfileHandler) parseFilterParams(r *http.Request, f *repository.ProfileFilter) error {
	q := r.URL.Query()

	if gn := q.Get("gender"); gn != "" {
		f.Gender = &gn
	}
	if ag := q.Get("age_group"); ag != "" {
		f.AgeGroup = &ag
	}
	if cid := q.Get("country_id"); cid != "" {
		f.CountryID = &cid
	}

	parseOptionalInt := func(key string, target **int) error {
		if val := q.Get(key); val != "" {
			n, err := strconv.Atoi(val)
			if err != nil {
				return err
			}
			*target = &n
		}
		return nil
	}
	parseOptionalFloat := func(key string, target **float64) error {
		if val := q.Get(key); val != "" {
			n, err := strconv.ParseFloat(val, 64)
			if err != nil {
				return err
			}
			*target = &n
		}
		return nil
	}

	if err := parseOptionalInt("min_age", &f.MinAge); err != nil {
		return err
	}
	if err := parseOptionalInt("max_age", &f.MaxAge); err != nil {
		return err
	}
	if err := parseOptionalFloat("min_gender_probability", &f.MinGenderProb); err != nil {
		return err
	}
	if err := parseOptionalFloat("min_country_probability", &f.MinCountryProb); err != nil {
		return err
	}

	f.SortBy = q.Get("sort_by")
	if f.SortBy != "" && f.SortBy != "age" && f.SortBy != "created_at" && f.SortBy != "gender_probability" {
		return fmt.Errorf("invalid sort_by")
	}

	f.Order = q.Get("order")
	if f.Order != "" && f.Order != "asc" && f.Order != "desc" {
		return fmt.Errorf("invalid order")
	}

	f.Page = 1
	if pgText := q.Get("page"); pgText != "" {
		pg, err := strconv.Atoi(pgText)
		if err != nil || pg <= 0 {
			return fmt.Errorf("invalid page")
		}
		f.Page = pg
	}

	f.Limit = 10
	if limText := q.Get("limit"); limText != "" {
		lim, err := strconv.Atoi(limText)
		if err != nil || lim <= 0 {
			return fmt.Errorf("invalid limit")
		}
		if lim > 50 {
			lim = 50 // clamp to 50
		}
		f.Limit = lim
	}

	return nil
}

func (h *ProfileHandler) List(w http.ResponseWriter, r *http.Request) {
	var filter repository.ProfileFilter
	if err := h.parseFilterParams(r, &filter); err != nil {
		response.Error(w, http.StatusUnprocessableEntity, "Invalid query parameters")
		return
	}

	profiles, total, err := h.svc.ListProfiles(r.Context(), filter)
	if err != nil {
		slog.Error("ListProfiles error", "error", err)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}

	response.PaginatedList(w, filter.Page, filter.Limit, total, profiles)
}

func (h *ProfileHandler) Search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		response.Error(w, http.StatusBadRequest, "Missing or empty parameter")
		return
	}

	filter, err := h.parser.ParseSearchQuery(query)
	if err != nil {
		response.Error(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	// Apply pagination overrides if any
	// Parse limit and page specifically so user can append ?limit=5
	if pgText := r.URL.Query().Get("page"); pgText != "" {
		if pg, perr := strconv.Atoi(pgText); perr == nil && pg > 0 {
			filter.Page = pg
		} else {
			response.Error(w, http.StatusUnprocessableEntity, "Invalid query parameters")
			return
		}
	}
	if limText := r.URL.Query().Get("limit"); limText != "" {
		if lim, perr := strconv.Atoi(limText); perr == nil && lim > 0 {
			if lim > 50 {
				lim = 50
			}
			filter.Limit = lim
		} else {
			response.Error(w, http.StatusUnprocessableEntity, "Invalid query parameters")
			return
		}
	}

	profiles, total, err := h.svc.ListProfiles(r.Context(), filter)
	if err != nil {
		slog.Error("ListProfiles search error", "error", err)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}

	response.PaginatedList(w, filter.Page, filter.Limit, total, profiles)
}

func (h *ProfileHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		response.Error(w, http.StatusNotFound, "Profile not found")
		return
	}

	err := h.svc.DeleteProfile(r.Context(), id)
	if err != nil {
		if err.Error() == "not found" {
			response.Error(w, http.StatusNotFound, "Profile not found")
			return
		}
		slog.Error("DeleteProfile error", "error", err, "id", id)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
