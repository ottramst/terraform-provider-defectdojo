package provider

import (
	"context"
	"math/big"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Raw states written by doximity/defectdojo v0.0.13 against DefectDojo
// 3.5.103, and one of v0.0.12, which named life_cycle lifecycle and had no
// critical_product and key_product.
const (
	doximityProductTypeState = `{"critical_product": false, "description": "pt desc", "id": "51", "key_product": false, "name": "spike-doximity-pt"}`

	doximityProductState = `{"business_criticality": "high", "description": "p desc", "enable_full_risk_acceptance": false,
		"enable_skip_risk_acceptance": true, "external_audience": true, "id": "69", "internet_accessible": true, "life_cycle": "production",
		"name": "spike-doximity-p", "origin": "internal", "platform": "web", "prod_numeric_grade": 3, "product_manager_id": null,
		"product_type_id": 51, "regulation_ids": [1], "revenue": "100.00", "tags": ["abc", "def"], "team_manager_id": null,
		"technical_contact_id": null, "user_records": 0}`

	doximityProductStateV0012 = `{"business_criticality": "", "description": "p desc", "enable_full_risk_acceptance": false,
		"enable_skip_risk_acceptance": false, "external_audience": false, "id": "70", "internet_accessible": false, "lifecycle": "",
		"name": "old", "origin": null, "platform": null, "prod_numeric_grade": null, "product_manager_id": null, "product_type_id": 51,
		"regulation_ids": null, "revenue": null, "tags": null, "team_manager_id": null, "technical_contact_id": null, "user_records": null}`

	doximityProductTypeStateV0012 = `{"description": "", "id": "52", "name": "old"}`

	doximityJiraState = `{"enable_engagement_epic_mapping": false, "engagement_id": null, "id": "4", "issue_template_dir": null,
		"jira_instance_id": "4", "product_id": "69", "product_jira_sla_notification": false, "project_key": "SPK", "push_all_issues": false,
		"push_notes": true, "risk_acceptance_expiration_notification": false}`
)

func testProviderServer(t *testing.T) (tfprotov6.ProviderServer, *tfprotov6.GetProviderSchemaResponse) {
	t.Helper()
	server, err := providerserver.NewProtocol6WithError(New("test")())()
	if err != nil {
		t.Fatal(err)
	}
	schemas, err := server.GetProviderSchema(context.Background(), &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}
	return server, schemas
}

// stateID returns the id of a state of type typ.
func stateID(t *testing.T, state *tfprotov6.DynamicValue, typ tftypes.Type) *big.Float {
	t.Helper()
	value, err := state.Unmarshal(typ)
	if err != nil {
		t.Fatal(err)
	}
	var attributes map[string]tftypes.Value
	if err := value.As(&attributes); err != nil {
		t.Fatal(err)
	}
	var id big.Float
	if err := attributes["id"].As(&id); err != nil {
		t.Fatal(err)
	}
	return &id
}

// The product types and products of doximity/defectdojo keep their type
// names here, so Terraform reads their states with this provider once the
// provider source changes: the states of schema version 0 must keep
// upgrading, although their IDs are strings. A future schema version must
// keep accepting them.
func TestUpgradeDoximityState(t *testing.T) {
	ctx := context.Background()
	server, schemas := testProviderServer(t)
	tests := []struct {
		typeName, state string
		id              int64
	}{
		{"defectdojo_product_type", doximityProductTypeState, 51},
		{"defectdojo_product_type", doximityProductTypeStateV0012, 52},
		{"defectdojo_product", doximityProductState, 69},
		{"defectdojo_product", doximityProductStateV0012, 70},
	}
	for _, tt := range tests {
		t.Run(tt.typeName, func(t *testing.T) {
			resp, err := server.UpgradeResourceState(ctx, &tfprotov6.UpgradeResourceStateRequest{
				TypeName: tt.typeName, Version: 0, RawState: &tfprotov6.RawState{JSON: []byte(tt.state)},
			})
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range resp.Diagnostics {
				t.Fatalf("UpgradeResourceState: %s: %s", d.Summary, d.Detail)
			}
			id := stateID(t, resp.UpgradedState, schemas.ResourceSchemas[tt.typeName].ValueType())
			if got, _ := id.Int64(); got != tt.id {
				t.Errorf("id = %s, want %d", id, tt.id)
			}
		})
	}
}

func TestMoveFromDoximity(t *testing.T) {
	ctx := context.Background()
	server, schemas := testProviderServer(t)
	identities, err := server.GetResourceIdentitySchemas(ctx, &tfprotov6.GetResourceIdentitySchemasRequest{})
	if err != nil {
		t.Fatal(err)
	}
	const target = "defectdojo_jira_project"
	tests := []struct {
		name, address, sourceType string
		version                   int64
		state                     string
		wantError                 string // "" for a move
	}{
		{"registry", "registry.terraform.io/doximity/defectdojo", "defectdojo_jira_product_configuration", 0, doximityJiraState, ""},
		{"no hostname", "doximity/defectdojo", "defectdojo_jira_product_configuration", 0, doximityJiraState, ""},
		{"other provider", "registry.terraform.io/other/defectdojo", "defectdojo_jira_product_configuration", 0, doximityJiraState, "Unable to Move Resource State"},
		{"other type", "registry.terraform.io/doximity/defectdojo", "defectdojo_product", 0, doximityJiraState, "Unable to Move Resource State"},
		{"other version", "registry.terraform.io/doximity/defectdojo", "defectdojo_jira_product_configuration", 1, doximityJiraState, "Unable to Move Resource State"},
		{"no numeric ID", "registry.terraform.io/doximity/defectdojo", "defectdojo_jira_product_configuration", 0, `{"id": "abc"}`, `got: "abc"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := server.MoveResourceState(ctx, &tfprotov6.MoveResourceStateRequest{
				SourceProviderAddress: tt.address,
				SourceTypeName:        tt.sourceType,
				SourceSchemaVersion:   tt.version,
				SourceState:           &tfprotov6.RawState{JSON: []byte(tt.state)},
				TargetTypeName:        target,
			})
			if err != nil {
				t.Fatal(err)
			}
			var errs []string
			for _, d := range resp.Diagnostics {
				if d.Severity == tfprotov6.DiagnosticSeverityError {
					errs = append(errs, d.Summary+": "+d.Detail)
				}
			}
			if tt.wantError != "" {
				if len(errs) == 0 || !strings.Contains(strings.Join(errs, "\n"), tt.wantError) {
					t.Fatalf("errors = %q, want one containing %q", errs, tt.wantError)
				}
				return
			}
			if len(errs) > 0 {
				t.Fatalf("errors = %q", errs)
			}
			typ := schemas.ResourceSchemas[target].ValueType()
			if id, _ := stateID(t, resp.TargetState, typ).Int64(); id != 4 {
				t.Errorf("id = %d, want 4", id)
			}
			// Only the ID is moved: Read fills in the rest.
			value, err := resp.TargetState.Unmarshal(typ)
			if err != nil {
				t.Fatal(err)
			}
			var attributes map[string]tftypes.Value
			if err := value.As(&attributes); err != nil {
				t.Fatal(err)
			}
			if !attributes["project_key"].IsNull() {
				t.Errorf("project_key = %s, want null", attributes["project_key"])
			}
			identity, err := resp.TargetIdentity.IdentityData.Unmarshal(identities.IdentitySchemas[target].ValueType())
			if err != nil {
				t.Fatal(err)
			}
			if want := tftypes.NewValue(identities.IdentitySchemas[target].ValueType(),
				map[string]tftypes.Value{"id": tftypes.NewValue(tftypes.Number, 4)}); !identity.Equal(want) {
				t.Errorf("identity = %s, want %s", identity, want)
			}
		})
	}
}
