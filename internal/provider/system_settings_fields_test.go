package provider

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ottramst/go-defectdojo"
)

// jsonFields returns the JSON names of the fields of a struct type.
func jsonFields(typ reflect.Type) map[string]bool {
	names := make(map[string]bool)
	for i := range typ.NumField() {
		name, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
		if name != "" && name != "-" {
			names[name] = true
		}
	}
	return names
}

// TestSystemSettingsFields checks the table against the client: every setting
// must be a field of SystemSettings and SystemSettingsPatch, since
// unmarshalling would silently drop an unknown name, and every writable field
// must be a setting, a secret, or read_only_mode, which is left out on
// purpose.
func TestSystemSettingsFields(t *testing.T) {
	read := jsonFields(reflect.TypeFor[defectdojo.SystemSettings]())
	write := jsonFields(reflect.TypeFor[defectdojo.SystemSettingsPatch]())
	covered := map[string]bool{"read_only_mode": true}
	seen := make(map[string]bool)
	for _, s := range systemSettingsFields {
		if seen[s.name] {
			t.Errorf("%s is in the table twice", s.name)
		}
		seen[s.name] = true
		if !read[s.jsonName()] || !write[s.jsonName()] {
			t.Errorf("%s: %q is not a field of SystemSettings and SystemSettingsPatch", s.name, s.jsonName())
		}
		covered[s.jsonName()] = true
	}
	for _, s := range systemSettingsSecrets {
		if !read[s.name] || !write[s.name] {
			t.Errorf("secret %q is not a field of SystemSettings and SystemSettingsPatch", s.name)
		}
		covered[s.name] = true
	}
	for name := range write {
		if !covered[name] {
			t.Errorf("the system setting %q is missing from the table", name)
		}
	}
}

// TestSystemSettingsPatch checks that values by JSON name become the patch's
// fields, and that a read value maps back.
func TestSystemSettingsPatch(t *testing.T) {
	patch, err := systemSettingsPatch(map[string]any{
		"team_name": "", "max_dupes": int64(0), "enable_jira": false, "allow_anonymous_survey_repsonse": true, //nolint:misspell // DefectDojo's JSON name.
		"slack_token": "xoxb-fake",
	})
	if err != nil {
		t.Fatal(err)
	}
	if patch.TeamName == nil || *patch.TeamName != "" || patch.EnableJira == nil || *patch.EnableJira ||
		patch.AllowAnonymousSurveyResponse == nil || !*patch.AllowAnonymousSurveyResponse || patch.SlackToken == nil || *patch.SlackToken != "xoxb-fake" {
		t.Errorf("patch = %+v", patch)
	}
	if v, ok := patch.MaxDupes.Get(); !ok || v != 0 {
		t.Errorf("MaxDupes = %v, %v; want 0", v, ok)
	}
	if patch.EmailFrom != nil || !patch.JiraLabels.IsZero() {
		t.Errorf("patch = %+v, want the other settings left out", patch)
	}

	values, err := systemSettingsValues(&defectdojo.SystemSettings{ID: 1, TeamName: "AppSec", ProductGradeA: 90, EnableJira: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range systemSettingsFields {
		v, err := s.value(values)
		if err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		switch s.name {
		case "team_name":
			if v.String() != `"AppSec"` {
				t.Errorf("team_name = %s", v)
			}
		case "product_grade_a":
			if v.String() != "90" {
				t.Errorf("product_grade_a = %s", v)
			}
		case "enable_jira":
			if v.String() != "true" {
				t.Errorf("enable_jira = %s", v)
			}
		case "max_dupes":
			if !v.IsNull() {
				t.Errorf("max_dupes = %s, want null", v)
			}
		}
	}
}
