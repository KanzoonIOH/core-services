package handler

import (
	"encoding/json"
	"testing"
)

func TestPluckField(t *testing.T) {
	var body map[string]any
	if err := json.Unmarshal([]byte(`{
		"output": "plain",
		"a.b": "dotted key",
		"choices": [{"message": {"content": "hello"}}]
	}`), &body); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		path string
		want any
	}{
		{"output", "plain"},
		{"a.b", "dotted key"},
		{"choices[0].message.content", "hello"},
		{"choices[3].message.content", nil}, // index out of range
		{"choices[0].missing", nil},
		{"nope", nil},
	}
	for _, c := range cases {
		if got := pluckField(body, c.path); got != c.want {
			t.Errorf("pluckField(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}
