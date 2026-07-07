package eggvars

import "testing"

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		rules   string
		wantErr bool
	}{
		{"empty rules always pass", "anything", "", false},
		{"required with value", "1.20.4", "required|string", false},
		{"required but empty", "", "required|string", true},
		{"optional and empty", "", "string|max:20", false},
		{"max ok", "short", "required|string|max:20", false},
		{"max exceeded", "this value is way too long for the limit", "required|string|max:20", true},
		{"min ok", "hello", "required|string|min:3", false},
		{"min not met", "hi", "required|string|min:3", true},
		{"integer ok", "42", "required|integer", false},
		{"integer bad", "not-a-number", "required|integer", true},
		{"numeric ok", "3.14", "required|numeric", false},
		{"numeric bad", "abc", "required|numeric", true},
		{"in allowed", "TRUE", "required|in:TRUE,FALSE", false},
		{"in not allowed", "MAYBE", "required|in:TRUE,FALSE", true},
		{"boolean ok", "true", "required|boolean", false},
		{"boolean bad", "yes", "required|boolean", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate("Field", tc.value, tc.rules)
			if tc.wantErr && err == nil {
				t.Fatalf("Validate(%q, %q) = nil, want an error", tc.value, tc.rules)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Validate(%q, %q) = %v, want nil", tc.value, tc.rules, err)
			}
		})
	}
}
