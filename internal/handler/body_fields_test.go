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

// On PATCH, an omitted key must leave the stored value alone (nil -> SQL NULL
// -> COALESCE keeps the column) while an explicit [] clears it. This is what
// stops the Persona/Customization tabs from wiping stored auth headers.
func TestOnUpdateHelpersDistinguishOmittedFromEmpty(t *testing.T) {
	if marshalBodyFieldsOnUpdate(nil) != nil {
		t.Fatal("omitted body fields must marshal to nil (preserve)")
	}
	if string(marshalBodyFieldsOnUpdate([]BodyField{})) != "[]" {
		t.Fatal("explicit [] must clear")
	}
	if trimNonEmptyOnUpdate(nil) != nil {
		t.Fatal("omitted origins must be nil (preserve)")
	}
	if got := trimNonEmptyOnUpdate([]string{}); got == nil || len(got) != 0 {
		t.Fatalf("explicit [] must clear, got %#v", got)
	}
	ips, err := parseAllowedIPsOnUpdate(nil)
	if err != nil || ips != nil {
		t.Fatalf("omitted ips must be nil (preserve), got %#v %v", ips, err)
	}
	if _, err := parseAllowedIPsOnUpdate([]string{"nope"}); err == nil {
		t.Fatal("invalid ip must still be rejected")
	}
	if trimStringOnUpdate(nil) != nil {
		t.Fatal("omitted guardrail must be nil (preserve)")
	}
	blank := "   "
	if got := trimStringOnUpdate(&blank); got == nil || *got != "" {
		t.Fatalf("explicit blank must clear, got %#v", got)
	}
}

func TestStoredAttachmentsPrefersDetails(t *testing.T) {
	details := []any{map[string]any{"name": "spec.txt", "url": "u"}}
	got := storedAttachments(map[string]any{
		"attachments":         []any{"u"},
		"attachments_details": details,
	})
	if _, ok := got.([]any)[0].(map[string]any); !ok {
		t.Fatalf("want the detailed form, got %#v", got)
	}
	// Clients that only send plain links still get persisted.
	got = storedAttachments(map[string]any{"attachments": []any{"u"}})
	if s, ok := got.([]any)[0].(string); !ok || s != "u" {
		t.Fatalf("want the link list fallback, got %#v", got)
	}
	if storedAttachments(map[string]any{}) != nil {
		t.Fatal("no attachments must stay nil")
	}
}

func TestApplyWebhookBodyFieldsAndHeaders(t *testing.T) {
	cfg := json.RawMessage(`[
		{"key":"tenant","type":"static","value":"ioh"},
		{"key":"user_id","type":"dynamic","value":""}
	]`)

	body := map[string]any{"chatInput": "hi", "user_id": "u1"}
	if msg := applyWebhookBodyFields(body, cfg); msg != "" {
		t.Fatalf("unexpected error: %s", msg)
	}
	if body["tenant"] != "ioh" || body["user_id"] != "u1" {
		t.Fatalf("body not injected correctly: %#v", body)
	}

	if msg := applyWebhookBodyFields(map[string]any{}, cfg); msg == "" {
		t.Fatal("missing dynamic field must be rejected")
	}

	hdrCfg := json.RawMessage(`[
		{"key":"Authorization","type":"static","value":"Bearer xyz"},
		{"key":"X-Tenant","type":"dynamic","value":""}
	]`)

	caller := popCallerHeaders(map[string]any{
		"chatInput": "hi",
		"headers":   map[string]any{"X-Tenant": "ioh"},
	})
	got, msg := resolveWebhookHeaders(hdrCfg, caller)
	if msg != "" {
		t.Fatalf("unexpected error: %s", msg)
	}
	if got["Authorization"] != "Bearer xyz" || got["X-Tenant"] != "ioh" {
		t.Fatalf("headers wrong: %#v", got)
	}

	if _, msg := resolveWebhookHeaders(hdrCfg, map[string]string{}); msg == "" {
		t.Fatal("missing dynamic header must be rejected")
	}

	// The reserved key must never survive into the forwarded body.
	body = map[string]any{"chatInput": "hi", "headers": map[string]any{"a": "b"}}
	popCallerHeaders(body)
	if _, ok := body["headers"]; ok {
		t.Fatal("reserved headers key must be stripped from the body")
	}
}
