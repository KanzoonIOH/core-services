package handler

import "testing"

func TestExtractSSEText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"json delta", "data: {\"delta\":\"He\"}\ndata: {\"delta\":\"llo\"}\n", "Hello"},
		{"text field", "data: {\"text\":\"hi\"}\n\n", "hi"},
		{"content field", "data: {\"content\":\"yo\"}\n", "yo"},
		{"done sentinel ignored", "data: {\"delta\":\"x\"}\ndata: [DONE]\n", "x"},
		{"raw non-json payload", "data: plain text\n", "plain text"},
		{"non-data lines skipped", "event: ping\ndata: {\"delta\":\"z\"}\n", "z"},
		{"done event wins, no dup", "data: {\"event\":\"token\",\"text\":\"Hello\"}\ndata: {\"reply\":\"Hello\",\"answer\":\"Hello\",\"event\":\"done\"}\n", "Hello"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		if got := extractSSEText([]byte(c.in)); got != c.want {
			t.Errorf("%s: extractSSEText(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}
