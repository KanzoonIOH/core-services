package handler

import (
	"encoding/json"
	"testing"
)

func TestMarshalBodyFields(t *testing.T) {
	in := []BodyField{
		{Key: "  user_id ", Type: "dynamic", Value: ""},
		{Key: "channel", Type: "static", Value: "whatsapp"},
		{Key: "", Type: "static", Value: "dropped"},   // blank key dropped
		{Key: "region", Type: "weird", Value: "apac"}, // bad type -> static
	}
	raw := marshalBodyFields(in)

	var out []BodyField
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("want 3 fields, got %d: %s", len(out), raw)
	}
	if out[0].Key != "user_id" || out[0].Type != "dynamic" {
		t.Fatalf("field 0 wrong: %+v", out[0])
	}
	if out[2].Type != "static" {
		t.Fatalf("bad type not defaulted to static: %+v", out[2])
	}
	// empty input must still be a valid JSON array, never null.
	if string(marshalBodyFields(nil)) != "[]" {
		t.Fatalf("nil input should marshal to []")
	}
}
