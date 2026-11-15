package provider

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
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
		{"vertical tab\v", false},
		{"\x1cfile separator", false},
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

func TestTagSet(t *testing.T) {
	strs := func(s ...string) types.Set {
		elems := make([]attr.Value, len(s))
		for i, v := range s {
			elems[i] = types.StringValue(v)
		}
		return types.SetValueMust(types.StringType, elems)
	}
	tests := []struct {
		name      string
		tags      []string
		reference types.Set
		want      types.Set
	}{
		{name: "none", tags: nil, reference: types.SetNull(types.StringType), want: strs()},
		{name: "no reference", tags: []string{"a", "B"}, reference: types.SetUnknown(types.StringType), want: strs("a", "B")},
		// Write responses can list a tag twice.
		{name: "duplicates", tags: []string{"own", "x", "own", "X"}, reference: types.SetNull(types.StringType), want: strs("own", "x")},
		// DefectDojo reused the spelling of an existing tag.
		{name: "reference spelling", tags: []string{"pci", "shop"}, reference: strs("PCI", "other"), want: strs("PCI", "shop")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tagSet(tt.tags, tt.reference); !got.Equal(tt.want) {
				t.Errorf("tagSet() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTagValidators(t *testing.T) {
	tests := []struct {
		tags    []string
		wantErr bool
	}{
		{tags: []string{"pci", "Shop", "a-b_c.d"}},
		{tags: []string{"foo bar"}, wantErr: true},
		{tags: []string{"a,b"}, wantErr: true},
		{tags: []string{`it's`}, wantErr: true},
		{tags: []string{""}, wantErr: true},
		{tags: []string{" x"}, wantErr: true},
		{tags: []string{string(make([]byte, 256))}, wantErr: true},
		{tags: []string{"X", "x"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.tags), func(t *testing.T) {
			elems := make([]attr.Value, len(tt.tags))
			for i, v := range tt.tags {
				elems[i] = types.StringValue(v)
			}
			req := validator.SetRequest{ConfigValue: types.SetValueMust(types.StringType, elems)}
			var diags diag.Diagnostics
			for _, v := range tagValidators() {
				resp := &validator.SetResponse{}
				v.ValidateSet(context.Background(), req, resp)
				diags.Append(resp.Diagnostics...)
			}
			if diags.HasError() != tt.wantErr {
				t.Errorf("errors = %v, want error: %v", diags, tt.wantErr)
			}
		})
	}
}

// The values DefectDojo returns unchanged are valid; it would rewrite or
// reject the others (checked against DefectDojo 3.5.103).
func TestRevenueValidator(t *testing.T) {
	tests := []struct {
		value string
		valid bool
	}{
		{"100.00", true},
		{"0.00", true},
		{"100.50", true},
		{"1234567890123.45", true},
		{"-1.00", false},             // a 400: at least 0.00
		{"-0.00", false},             // accepted, but no minus sign is needed
		{"100", false},               // 100.00
		{"100.5", false},             // 100.50
		{"0012.30", false},           // 12.30
		{"1e3", false},               // 1000.00
		{"-0", false},                // -0.00
		{"12345678901234.00", false}, // more than 15 digits: a 400
		{"", false},
	}
	v := revenueValidator()
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			req := validator.StringRequest{ConfigValue: types.StringValue(tt.value)}
			var resp validator.StringResponse
			v.ValidateString(context.Background(), req, &resp)
			if valid := !resp.Diagnostics.HasError(); valid != tt.valid {
				t.Errorf("valid = %v, want %v", valid, tt.valid)
			}
		})
	}
}

// DefectDojo stores these integers in 32-bit columns; 0 and negative numbers
// are valid there.
func TestInt32Range(t *testing.T) {
	for value, valid := range map[int64]bool{
		0: true, -1: true, 10011: true, math.MaxInt32: true, math.MinInt32: true,
		math.MaxInt32 + 1: false, math.MinInt32 - 1: false,
	} {
		var resp validator.Int64Response
		int32Range().ValidateInt64(context.Background(), validator.Int64Request{ConfigValue: types.Int64Value(value)}, &resp)
		if resp.Diagnostics.HasError() == valid {
			t.Errorf("%d: valid = %t, want %t", value, !resp.Diagnostics.HasError(), valid)
		}
	}
}
