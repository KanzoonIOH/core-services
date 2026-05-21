package lib

import (
	"encoding/json"
	"math"
	"net/http"
)

func responseJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(payload)
}

type responseType struct {
	Message    *string `json:"message,omitempty"`
	Data       any     `json:"data,omitempty"`
	Pagination any     `json:"pagination,omitempty"`
}

func ResponseJSONTemplate(w http.ResponseWriter, status int, message *string, data any, pagination any) {
	responseJSON(w, status, responseType{
		Message:    message,
		Data:       data,
		Pagination: pagination,
	})
}

func ResponseJSONError(w http.ResponseWriter, status int, message string) {
	ResponseJSONTemplate(w, status, &message, nil, nil)
}

func ResponsePagination(limit int, offset int, rows int, total_row int) map[string]any {
	return map[string]any{
		"limit":      limit,
		"page":       offset + 1,
		"rows":       rows,
		"total_page": math.Ceil(float64(total_row) / float64(limit)),
		"total_row":  total_row,
	}
}
