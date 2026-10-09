package utils

import "testing"

func TestQuote(t *testing.T) {
	value := "v2"
	tests := []struct {
		name string
		in   any
		want string
	}{
		{name: "plain", in: "frp.example.com", want: `"frp.example.com"`},
		{name: "empty", in: "", want: `""`},
		{name: "double quote", in: `pa"ss`, want: `"pa\"ss"`},
		{name: "backslash", in: `C:\path`, want: `"C:\\path"`},
		{name: "control characters", in: "a\tb\nc\rd\be\ff\x00g\x7f", want: `"a\tb\nc\rd\be\ff\u0000g\u007F"`},
		{name: "unicode is kept", in: "héllo ✓", want: `"héllo ✓"`},
		{name: "string pointer", in: &value, want: `"v2"`},
		{name: "nil string pointer", in: (*string)(nil), want: `""`},
		{name: "non-string", in: 42, want: `"42"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Quote(tt.in); got != tt.want {
				t.Errorf("Quote(%q) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}
