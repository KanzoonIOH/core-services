package lib

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

func ParseJSONBody(w http.ResponseWriter, r *http.Request, req any) bool {
	err := json.NewDecoder(r.Body).Decode(req)
	if err != nil {
		ResponseJSON(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func ParseID(w http.ResponseWriter, r *http.Request, id string) (uuid.UUID, bool) {
	v, err := uuid.Parse(chi.URLParam(r, id))
	if err != nil {
		ResponseJSON(w, http.StatusBadRequest, "invalid id")
		return uuid.Nil, false
	}

	return v, true
}

type paginationParams struct {
	Limit  int32
	Offset int32
	Sort   *string
}

func ParsePaginationParams(params url.Values) paginationParams {
	const (
		defaultLimit  = 10
		defaultOffset = 0
	)

	limit, err := strconv.Atoi(params.Get("limit"))
	if err != nil || limit <= 0 {
		limit = defaultLimit
	}

	offset, err := strconv.Atoi(params.Get("offset"))
	if err != nil || offset < 0 {
		offset = defaultOffset
	}

	var sort *string
	if s := params.Get("sort"); s != "" {
		sort = &s
	}

	return paginationParams{
		Limit:  int32(limit),
		Offset: int32(offset),
		Sort:   sort,
	}
}

func ParseQueryBool(params url.Values, name string) *bool {
	var v *bool
	if s := params.Get(name); s != "" {
		b, err := strconv.ParseBool(s)
		if err == nil {
			v = &b
		}
	}

	return v
}

func ParseParamsString(params url.Values, name string) *string {
	var v *string
	if s := params.Get(name); s != "" {
		v = &s
	}

	return v
}
