package provider

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/ottramst/go-defectdojo"
)

func TestParseImportID(t *testing.T) {
	tests := []struct {
		id     string
		want   int64
		wantOK bool
	}{
		{id: "42", want: 42, wantOK: true},
		{id: "0"},
		{id: "-1"},
		{id: "abc"},
		{id: "1/2"},
		{id: ""},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			var diags diag.Diagnostics
			got, ok := parseImportID(tt.id, "product type", &diags)
			if got != tt.want || ok != tt.wantOK || diags.HasError() == tt.wantOK {
				t.Errorf("parseImportID(%q) = %d, %v (diags %v); want %d, %v", tt.id, got, ok, diags, tt.want, tt.wantOK)
			}
		})
	}
}

func TestWaitForDeletion(t *testing.T) {
	deletePollInterval = time.Millisecond
	t.Cleanup(func() { deletePollInterval = time.Second })
	notFound := &defectdojo.ErrorResponse{StatusCode: 404}

	calls := 0
	err := waitForDeletion(context.Background(), func(context.Context) error {
		calls++
		if calls < 3 {
			return nil // still there
		}
		return notFound
	})
	if err != nil || calls != 3 {
		t.Errorf("waitForDeletion() = %v after %d calls, want nil after 3", err, calls)
	}

	boom := errors.New("boom")
	if err := waitForDeletion(context.Background(), func(context.Context) error { return boom }); !errors.Is(err, boom) {
		t.Errorf("waitForDeletion(error) = %v, want %v", err, boom)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := waitForDeletion(ctx, func(context.Context) error { return nil }); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("waitForDeletion(never deleted) = %v, want a deadline error", err)
	}
}

func TestStringOrNull(t *testing.T) {
	if !stringOrNull("").IsNull() {
		t.Error(`stringOrNull("") is not null`)
	}
	if got := stringOrNull("x").ValueString(); got != "x" {
		t.Errorf(`stringOrNull("x") = %q`, got)
	}
}

func TestInt64Set(t *testing.T) {
	if s := int64Set(nil); s.IsNull() || len(s.Elements()) != 0 {
		t.Errorf("int64Set(nil) = %v, want an empty set", s)
	}
	if got := fmt.Sprint(int64Set([]int{3, 1}).Elements()); got != "[1 3]" && got != "[3 1]" {
		t.Errorf("int64Set([3 1]) = %s", got)
	}
}

func TestNoSurroundingWhitespace(t *testing.T) {
	tests := []struct {
		value string
		valid bool
	}{
		{"Web Applications", true},
		{"a", true},
		{"first line\nsecond line", true},
		{"Tab\tinside", true},
		{" leading", false},
		{"trailing ", false},
		{"heredoc\n", false},
		{"\tTab", false},
		{"no-break space ", false},
		{"　ideographic space", false},
		{"line separator ", false},
		{"next line\u0085", false},
	}
	v := noSurroundingWhitespace()
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			req := validator.StringRequest{ConfigValue: types.StringValue(tt.value)}
			var resp validator.StringResponse
			v.ValidateString(context.Background(), req, &resp)
			if got := !resp.Diagnostics.HasError(); got != tt.valid {
				t.Errorf("valid = %v, want %v (%v)", got, tt.valid, resp.Diagnostics)
			}
		})
	}
}
