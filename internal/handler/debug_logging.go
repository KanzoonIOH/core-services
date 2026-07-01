package handler

import (
	"encoding/json"
	"fmt"
)

func jsonForLog(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%+v", v)
	}
	return string(b)
}

func truncateForLog(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return fmt.Sprintf("%s...<truncated %d bytes>", s[:max], len(s)-max)
}
