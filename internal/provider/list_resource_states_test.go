package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// listFixtures are the objects of the fake DefectDojo of the list resource
// tests, by collection. The engagements' product passes its tag "prod" on.
var listFixtures = map[string][]map[string]any{
	"product_types": {
		{"id": 1, "name": "Web", "description": "Web applications", "critical_product": true, "key_product": false, "authorized_users": []any{1}},
	},
	"products": {
		{
			"id": 1, "name": "Shop", "description": "The shop", "prod_type": 1, "business_criticality": "high", "platform": nil,
			"lifecycle": "production", "origin": nil, "user_records": 0, "revenue": "100.00", "external_audience": true,
			"internet_accessible": false, "enable_product_tag_inheritance": true, "enable_simple_risk_acceptance": false,
			"enable_full_risk_acceptance": true, "disable_sla_breach_notifications": false, "sla_configuration": 1,
			"product_manager": 1, "technical_contact": nil, "team_manager": nil,
			"authorized_users": []any{}, "regulations": []any{2}, "tags": []any{"prod"},
		},
	},
	"engagements": {
		{
			"id": 1, "name": "CI", "product": 1, "target_start": "2026-11-01", "target_end": "2026-12-31", "first_contacted": nil,
			"engagement_type": "CI/CD", "status": "In Progress", "active": true, "description": "Scans", "reason": nil,
			"version": "1.4.0", "build_id": nil, "commit_hash": nil, "branch_tag": "main", "tracker": nil, "test_strategy": nil,
			"source_code_management_uri": nil, "deduplication_on_engagement": false, "threat_model": true, "api_test": true,
			"pen_test": true, "check_list": true, "lead": 1, "cicd_scm_server": nil, "cicd_build_server": 3,
			"cicd_orchestration_engine": nil, "tags": []any{"ci", "prod"},
		},
		{
			"id": 2, "name": "Pentest", "product": 1, "target_start": "2026-11-01", "target_end": "2026-11-30",
			"engagement_type": "Interactive", "status": "Not Started", "active": true, "tags": []any{"prod"},
		},
	},
	"users": {
		{
			"id": 7, "username": "dependency-track", "first_name": "Dependency", "last_name": "", "email": "dt@example.com",
			"date_joined": "2026-11-14T14:18:16.110685Z", "is_active": true, "is_staff": false, "is_superuser": false,
			"configuration_permissions": []any{9},
		},
		{
			"id": 8, "username": "reader", "email": "reader@example.com", "date_joined": "2026-11-14T14:18:16.110685Z",
			"is_active": true, "configuration_permissions": []any{},
		},
	},
	"configuration_permissions": {
		{"id": 9, "name": "Can add product type", "codename": "add_product_type"},
	},
	"sla_configurations": {
		{
			"id": 18, "name": "Strict", "description": "Internet-facing products", "critical": 3, "enforce_critical": true, "high": 14,
			"enforce_high": true, "medium": 60, "enforce_medium": true, "low": 90, "enforce_low": false, "restart_sla_on_reactivation": true,
		},
	},
	"jira_instances": {
		{
			"id": 1, "configuration_name": "", "url": "https://jira.example.com", "username": "defectdojo-bot", "default_issue_type": "Security",
			"issue_template_dir": nil, "epic_name_id": 10011, "open_status_key": 11, "close_status_key": 31, "info_mapping_severity": "Lowest",
			"low_mapping_severity": "Low", "medium_mapping_severity": "Medium", "high_mapping_severity": "High", "critical_mapping_severity": "Highest",
			"finding_text": "Reported by DefectDojo", "accepted_mapping_resolution": "Won't Fix", "false_positive_mapping_resolution": nil,
			"global_jira_sla_notification": true, "finding_jira_sync": false,
		},
	},
	"jira_projects": {
		{
			"id": 2, "project_key": "SEC", "issue_template_dir": nil, "component": "web,api",
			"custom_fields":           map[string]any{"customfield_10122": []any{map[string]any{"name": "8.0.1"}}},
			"close_transition_fields": map[string]any{"resolution": map[string]any{"name": "Won't Fix"}}, "reopen_transition_fields": nil,
			"default_assignee": nil, "jira_labels": "security defectdojo", "add_vulnerability_id_to_jira_label": false, "push_all_issues": true,
			"enable_engagement_epic_mapping": false, "epic_issue_type_name": "Epic", "push_notes": false, "product_jira_sla_notification": false,
			"suppress_sla_notification_comments": false, "risk_acceptance_expiration_notification": false, "enabled": true,
			"jira_instance": 1, "product": 1, "engagement": nil,
		},
		{
			"id": 3, "project_key": "PEN", "epic_issue_type_name": "Epic", "enabled": true, "jira_instance": 1, "product": nil, "engagement": 2,
		},
	},
	"notification_webhooks": {
		{"id": 5, "name": "Pager", "url": "https://pager.example.com", "header_name": nil, "header_value": nil, "status": "active", "owner": nil},
		{
			"id": 4, "name": "Chat relay", "url": "https://hooks.example.com/defectdojo", "header_name": "Authorization",
			"header_value": "Bearer fake-token", "status": "active", "first_error": nil, "last_error": nil, "note": "", "owner": nil,
		},
	},
	"notifications": {
		{
			"id": 2, "product": nil, "user": nil, "template": false, "product_type_added": []any{"alert"}, "product_added": []any{"alert", "mail"},
			"engagement_added": []any{}, "scan_added": []any{"alert"}, "scan_added_empty": []any{},
		},
		{"id": 3, "product": nil, "user": nil, "template": true, "scan_added": []any{"mail"}},
		{"id": 4, "product": 1, "user": 7, "template": false},
	},
	"regulations": {
		{
			"id": 29, "name": "GDPR Test", "acronym": "GDPR-T", "category": "privacy", "jurisdiction": "European Union",
			"description": "A test regulation.", "reference": "https://example.com/gdpr",
		},
	},
	"note_type": {
		{"id": 6, "name": "Sign-off", "description": "The release sign-off.", "is_single": true, "is_active": false, "is_mandatory": true},
	},
	"development_environments": {
		{"id": 12, "name": "QA"},
	},
	"cicd_infrastructure": {
		{"id": 8, "name": "Jenkins", "description": "The main build server.", "url": "https://jenkins.example.com", "infrastructure_type": "build_server"},
	},
	"product_platforms": {
		{"id": 9, "value": "tv", "name": "TV", "icon": "", "display_order": 0},
		{"id": 8, "value": "kiosk", "name": "Kiosk", "icon": "tv", "display_order": 60},
	},
	"product_lifecycles": {
		{"id": 1, "value": "construction", "name": "Construction", "icon": "", "display_order": 10},
	},
	"product_origins": {
		{"id": 1, "value": "internal", "name": "Internal", "icon": "", "display_order": 10},
	},
}

// listFake serves listFixtures: lists of each collection (without filters)
// and its objects by ID, and counts the GET requests by path.
type listFake struct {
	mu   sync.Mutex
	gets map[string]int
}

// count returns the number of GET requests of path.
func (f *listFake) count(path string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.gets[path]
}

func (f *listFake) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.gets[r.URL.Path]++
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v2/"), "/"), "/")
		objects, ok := listFixtures[parts[0]]
		status, body := http.StatusNotFound, any(map[string]string{"detail": "No object matches the given query."})
		switch {
		case !ok || len(parts) > 2:
		case len(parts) == 1:
			status, body = http.StatusOK, map[string]any{"count": len(objects), "next": nil, "previous": nil, "results": objects}
		default:
			for _, o := range objects {
				if fmt.Sprint(o["id"]) == parts[1] {
					status, body = http.StatusOK, o
				}
			}
		}
		w.WriteHeader(status)
		if err := json.NewEncoder(w).Encode(body); err != nil {
			t.Error(err)
		}
	})
}

// TestListResourceStates checks that every list resource stores the state an
// import followed by Read stores, so that the configuration terraform query
// generates from it plans no changes, and that the engagements read their
// product once.
func TestListResourceStates(t *testing.T) {
	fake := &listFake{gets: map[string]int{}}
	srv := httptest.NewServer(fake.handler(t))
	defer srv.Close()
	t.Setenv("DEFECTDOJO_URL", srv.URL)
	t.Setenv("DEFECTDOJO_API_TOKEN", "token")

	ctx := context.Background()
	server, err := providerserver.NewProtocol6WithError(New("test")())()
	if err != nil {
		t.Fatal(err)
	}
	schemas, err := server.GetProviderSchema(ctx, &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}
	providerType, ok := schemas.Provider.ValueType().(tftypes.Object)
	if !ok {
		t.Fatalf("the provider schema is a %T", schemas.Provider.ValueType())
	}
	config, err := tfprotov6.NewDynamicValue(providerType, nullAttributes(providerType, nil))
	if err != nil {
		t.Fatal(err)
	}
	configureResp, err := server.ConfigureProvider(ctx, &tfprotov6.ConfigureProviderRequest{Config: &config})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range configureResp.Diagnostics {
		t.Fatalf("ConfigureProvider: %s: %s", d.Summary, d.Detail)
	}

	tests := map[string]struct {
		collection   string
		displayNames []string
	}{
		"defectdojo_product_type": {"product_types", []string{"Web"}},
		"defectdojo_product":      {"products", []string{"Shop"}},
		"defectdojo_engagement":   {"engagements", []string{"Shop / CI", "Shop / Pentest"}},
		"defectdojo_user":         {"users", []string{"dependency-track", "reader"}},

		"defectdojo_sla_configuration":       {"sla_configurations", []string{"Strict"}},
		"defectdojo_jira_instance":           {"jira_instances", []string{"https://jira.example.com"}},
		"defectdojo_jira_project":            {"jira_projects", []string{"SEC / Shop", "PEN / Pentest"}},
		"defectdojo_notification_webhook":    {"notification_webhooks", []string{"Chat relay", "Pager"}},
		"defectdojo_notifications":           {"notifications", []string{"system-wide", "template", "user 7, product 1"}},
		"defectdojo_regulation":              {"regulations", []string{"GDPR Test"}},
		"defectdojo_note_type":               {"note_type", []string{"Sign-off"}},
		"defectdojo_development_environment": {"development_environments", []string{"QA"}},
		"defectdojo_cicd_infrastructure":     {"cicd_infrastructure", []string{"Jenkins"}},
		"defectdojo_product_platform":        {"product_platforms", []string{"kiosk", "tv"}},
		"defectdojo_product_lifecycle":       {"product_lifecycles", []string{"construction"}},
		"defectdojo_product_origin":          {"product_origins", []string{"internal"}},
	}
	if len(tests) != len(schemas.ListResourceSchemas) {
		t.Errorf("%d list resources are tested, %d are registered", len(tests), len(schemas.ListResourceSchemas))
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			listSchema, ok := schemas.ListResourceSchemas[name]
			if !ok {
				t.Fatal("the list resource is not registered")
			}
			configType, ok := listSchema.ValueType().(tftypes.Object)
			if !ok {
				t.Fatalf("the list schema is a %T", listSchema.ValueType())
			}
			listConfig, err := tfprotov6.NewDynamicValue(configType, nullAttributes(configType, nil))
			if err != nil {
				t.Fatal(err)
			}
			lister, ok := server.(tfprotov6.ListResourceServer)
			if !ok {
				t.Fatal("the provider server can't list resources")
			}
			stream, err := lister.ListResource(ctx, &tfprotov6.ListResourceRequest{
				TypeName: name, Config: &listConfig, IncludeResource: true, Limit: 100,
			})
			if err != nil {
				t.Fatal(err)
			}
			typ, ok := schemas.ResourceSchemas[name].ValueType().(tftypes.Object)
			if !ok {
				t.Fatalf("the schema is a %T", schemas.ResourceSchemas[name].ValueType())
			}
			before := fake.count("/api/v2/products/1/")
			var results []tfprotov6.ListResourceResult
			for result := range stream.Results {
				results = append(results, result)
			}
			// The engagements read their product once.
			if got := fake.count("/api/v2/products/1/") - before; name == "defectdojo_engagement" && got != 1 {
				t.Errorf("the list read the product %d times, want 1", got)
			}
			var displayNames []string
			for _, result := range results {
				for _, d := range result.Diagnostics {
					t.Errorf("ListResource: %s: %s: %s", d.Severity, d.Summary, d.Detail)
				}
				if result.Resource == nil {
					t.Fatalf("result %q has no resource", result.DisplayName)
				}
				displayNames = append(displayNames, result.DisplayName)
				listed, err := result.Resource.Unmarshal(typ)
				if err != nil {
					t.Fatal(err)
				}
				var attributes map[string]tftypes.Value
				if err := listed.As(&attributes); err != nil {
					t.Fatal(err)
				}

				// An import sets the ID; Read reads the rest.
				imported, err := tfprotov6.NewDynamicValue(typ, nullAttributes(typ, map[string]tftypes.Value{"id": attributes["id"]}))
				if err != nil {
					t.Fatal(err)
				}
				read, err := server.ReadResource(ctx, &tfprotov6.ReadResourceRequest{TypeName: name, CurrentState: &imported})
				if err != nil {
					t.Fatal(err)
				}
				for _, d := range read.Diagnostics {
					t.Errorf("ReadResource: %s: %s: %s", d.Severity, d.Summary, d.Detail)
				}
				want, err := read.NewState.Unmarshal(typ)
				if err != nil {
					t.Fatal(err)
				}
				if !listed.Equal(want) {
					t.Errorf("result %q:\nlisted %s\nread   %s", result.DisplayName, listed, want)
				}
			}
			if fmt.Sprint(displayNames) != fmt.Sprint(tt.displayNames) {
				t.Errorf("display names = %q, want %q", displayNames, tt.displayNames)
			}
		})
	}
}

func TestContainsFold(t *testing.T) {
	tests := []struct {
		value  string
		filter types.String
		want   bool
	}{
		{"Research and Development", types.StringNull(), true},
		{"Research and Development", types.StringValue("and dev"), true},
		{"Research and Development", types.StringValue("RESEARCH"), true},
		{"Research and Development", types.StringValue("Research  and"), false},
		{"", types.StringValue("a"), false},
	}
	for _, tt := range tests {
		if got := containsFold(tt.value, tt.filter); got != tt.want {
			t.Errorf("containsFold(%q, %s) = %v, want %v", tt.value, tt.filter, got, tt.want)
		}
	}
}

// A result with an error ends the stream: the error would repeat for the
// objects that follow.
func TestListResultsErrorEndsStream(t *testing.T) {
	all := func(yield func(*int, error) bool) {
		for i := 1; i <= 3; i++ {
			if !yield(&i, nil) {
				return
			}
		}
	}
	var got []string
	for r := range listResults(all, func(i *int) int { return *i }, nil, func(i *int) list.ListResult {
		var diags diag.Diagnostics
		if *i == 2 {
			diags.AddError("failed", "")
		}
		return list.ListResult{DisplayName: fmt.Sprint(*i), Diagnostics: diags}
	}, listError("objects")) {
		got = append(got, r.DisplayName)
	}
	if fmt.Sprint(got) != "[1 2]" {
		t.Errorf("results = %q, want [1 2]", got)
	}
}
