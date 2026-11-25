package jsonx_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/urandom/on-exercise/internal/jsonx"
)

func TestDecodeStrict(t *testing.T) {
	type rec struct {
		ID string `json:"id"`
	}

	tests := []struct {
		name    string
		in      string
		target  func() any
		wantErr error // nil: success; errAny: any error
	}{
		{name: "struct array", in: `[{"id":"a"}]`, target: func() any { return &[]rec{} }},
		{name: "trailing whitespace", in: "[]\n\t ", target: func() any { return &[]rec{} }},
		{name: "map accepts any key", in: `{"x":["y"]}`, target: func() any { return &map[string][]string{} }},
		{name: "unknown struct field", in: `[{"id":"a","role":"x"}]`, target: func() any { return &[]rec{} }, wantErr: errAny},
		{name: "malformed", in: `[{"id":`, target: func() any { return &[]rec{} }, wantErr: errAny},
		{name: "second value", in: `[] []`, target: func() any { return &[]rec{} }, wantErr: jsonx.ErrTrailingData},
		{name: "stray characters", in: `[] x`, target: func() any { return &[]rec{} }, wantErr: jsonx.ErrTrailingData},
		{name: "unmatched bracket", in: `{}}`, target: func() any { return &map[string][]string{} }, wantErr: jsonx.ErrTrailingData},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := jsonx.DecodeStrict(strings.NewReader(tt.in), tt.target())
			switch {
			case tt.wantErr == nil && err != nil:
				t.Fatalf("DecodeStrict() error = %v, want nil", err)
			case tt.wantErr == errAny && err == nil:
				t.Fatal("DecodeStrict() succeeded, want error")
			case tt.wantErr != nil && tt.wantErr != errAny && !errors.Is(err, tt.wantErr):
				t.Fatalf("DecodeStrict() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// errAny marks cases where any error is acceptable.
var errAny = errors.New("any error")
