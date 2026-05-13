package handler

import (
	"aiac-service/internal/lib"
	"net/http"
)

func Health(w http.ResponseWriter, r *http.Request) {
	lib.ResponseJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
