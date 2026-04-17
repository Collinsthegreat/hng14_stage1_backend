package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/username/repo-name/internal/model"
	"github.com/username/repo-name/internal/service"
	"github.com/username/repo-name/pkg/response"
)

type ProfileHandler struct {
	svc service.ProfileService
}

func NewProfileHandler(svc service.ProfileService) *ProfileHandler {
	return &ProfileHandler{svc: svc}
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

func (h *ProfileHandler) List(w http.ResponseWriter, r *http.Request) {
	gender := r.URL.Query().Get("gender")
	countryID := r.URL.Query().Get("country_id")
	ageGroup := r.URL.Query().Get("age_group")

	profiles, err := h.svc.ListProfiles(r.Context(), gender, countryID, ageGroup)
	if err != nil {
		slog.Error("ListProfiles error", "error", err)
		response.Error(w, http.StatusInternalServerError, "internal server error")
		return
	}

	response.List(w, len(profiles), profiles)
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
