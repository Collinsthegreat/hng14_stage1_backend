package response

import (
	"encoding/json"
	"net/http"
)

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

type PaginatedResponse struct {
	Status string `json:"status"`
	Page   int    `json:"page"`
	Limit  int    `json:"limit"`
	Total  int    `json:"total"`
	Data   any    `json:"data"`
}

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

func PaginatedList(w http.ResponseWriter, page, limit, total int, data any) {
	JSON(w, http.StatusOK, PaginatedResponse{
		Status: "success",
		Page:   page,
		Limit:  limit,
		Total:  total,
		Data:   data,
	})
}

func Error(w http.ResponseWriter, statusCode int, message string) {
	JSON(w, statusCode, ErrorResponse{
		Status:  "error",
		Message: message,
	})
}
