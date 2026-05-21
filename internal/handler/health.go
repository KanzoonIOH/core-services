package handler

import (
	"aiac-service/internal/lib"
	"net/http"
)

func Health(w http.ResponseWriter, r *http.Request) {
	lib.ResponseJSONTemplate(w, http.StatusOK, nil, map[string]string{"status": "ok"}, nil)
}
