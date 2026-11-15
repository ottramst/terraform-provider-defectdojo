package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// TestReadGoneWithoutIdentity checks that every resource removes an object
// that is gone from a state without identity, as a state taken over from
// doximity/defectdojo or used with Terraform before 1.12 has, and sets the
// identity: the framework rejects a null identity after Read even when the
// state is removed ("Missing Resource Identity After Read").
func TestReadGoneWithoutIdentity(t *testing.T) {
	// Every object is gone; the system settings, which always exist, have
	// another ID than the state's.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v2/system_settings/" {
			_, _ = w.Write([]byte(`{"count": 1, "next": null, "previous": null, "results": [{"id": 1}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"detail": "No object matches the given query."}`))
	}))
	defer srv.Close()
	t.Setenv("DEFECTDOJO_URL", srv.URL)
	t.Setenv("DEFECTDOJO_API_TOKEN", "token")
	t.Setenv("DEFECTDOJO_USERNAME", "")
	t.Setenv("DEFECTDOJO_PASSWORD", "")

	ctx := context.Background()
	server, err := providerserver.NewProtocol6WithError(New("test")())()
	if err != nil {
		t.Fatal(err)
	}
	schemas, err := server.GetProviderSchema(ctx, &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}
	identities, err := server.GetResourceIdentitySchemas(ctx, &tfprotov6.GetResourceIdentitySchemasRequest{})
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

	for name, schema := range schemas.ResourceSchemas {
		t.Run(name, func(t *testing.T) {
			if _, ok := identities.IdentitySchemas[name]; !ok {
				t.Fatal("the resource has no identity schema")
			}
			typ, ok := schema.ValueType().(tftypes.Object)
			if !ok {
				t.Fatalf("the schema is a %T", schema.ValueType())
			}
			state, err := tfprotov6.NewDynamicValue(typ, nullAttributes(typ, map[string]tftypes.Value{"id": tftypes.NewValue(tftypes.Number, 7)}))
			if err != nil {
				t.Fatal(err)
			}
			resp, err := server.ReadResource(ctx, &tfprotov6.ReadResourceRequest{TypeName: name, CurrentState: &state})
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range resp.Diagnostics {
				if d.Severity == tfprotov6.DiagnosticSeverityError {
					t.Errorf("ReadResource: %s: %s", d.Summary, d.Detail)
				}
			}
			newState, err := resp.NewState.Unmarshal(typ)
			if err != nil {
				t.Fatal(err)
			}
			if !newState.IsNull() {
				t.Errorf("the state was not removed: %s", newState)
			}
		})
	}
}

// nullAttributes returns an object of type typ whose attributes are null,
// except those in values.
func nullAttributes(typ tftypes.Object, values map[string]tftypes.Value) tftypes.Value {
	attributes := map[string]tftypes.Value{}
	for k, v := range typ.AttributeTypes {
		attributes[k] = tftypes.NewValue(v, nil)
	}
	for k, v := range values {
		attributes[k] = v
	}
	return tftypes.NewValue(typ, attributes)
}
