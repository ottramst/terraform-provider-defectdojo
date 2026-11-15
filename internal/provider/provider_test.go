package provider

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/ottramst/go-defectdojo"
)

// testAccProtoV6ProviderFactories are used to instantiate a provider during
// acceptance testing. The factory function will be invoked for every Terraform
// CLI command executed to create a provider server to which the CLI can
// reattach.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"defectdojo": providerserver.NewProtocol6WithError(New("test")()),
}

// testAccPreCheck fails the test when the provider cannot be configured from
// the environment. `make testacc` sets the variables for the local stack.
func testAccPreCheck(t *testing.T) {
	t.Helper()
	if os.Getenv("DEFECTDOJO_URL") == "" {
		t.Fatal("DEFECTDOJO_URL must be set for acceptance tests; run `make up testacc`")
	}
	if os.Getenv("DEFECTDOJO_API_TOKEN") == "" && os.Getenv("DEFECTDOJO_USERNAME") == "" {
		t.Fatal("DEFECTDOJO_API_TOKEN, or DEFECTDOJO_USERNAME and DEFECTDOJO_PASSWORD, must be set for acceptance tests")
	}
}

// testAccSkipUnlessAcc skips a test unless TF_ACC is set, like
// resource.Test does. Tests that create objects before resource.Test call it
// first.
func testAccSkipUnlessAcc(t *testing.T) {
	t.Helper()
	if os.Getenv("TF_ACC") == "" {
		t.Skip("acceptance tests are skipped unless TF_ACC is set")
	}
}

// testAccClient returns a client configured like the provider, to change or
// check objects outside Terraform.
func testAccClient(t *testing.T) *defectdojo.Client {
	t.Helper()
	testAccPreCheck(t)
	auth := defectdojo.WithBasicAuth(os.Getenv("DEFECTDOJO_USERNAME"), os.Getenv("DEFECTDOJO_PASSWORD"))
	if token := os.Getenv("DEFECTDOJO_API_TOKEN"); token != "" {
		auth = defectdojo.WithAPIToken(token)
	}
	client, err := defectdojo.NewClient(os.Getenv("DEFECTDOJO_URL"), auth)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	return client
}

// testAccNoError fails the test on an error in a PreConfig or check function.
func testAccNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// randName returns a unique object name, "acc-" and a random suffix.
func randName(t *testing.T) string {
	t.Helper()
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return "acc-" + hex.EncodeToString(b)
}

func TestProvider_schemas(t *testing.T) {
	ctx := context.Background()
	p := New("test")()

	var schemaResp fwprovider.SchemaResponse
	p.Schema(ctx, fwprovider.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("provider schema: %v", schemaResp.Diagnostics)
	}
	if diags := schemaResp.Schema.ValidateImplementation(ctx); diags.HasError() {
		t.Fatalf("provider schema: %v", diags)
	}

	server, err := providerserver.NewProtocol6WithError(p)()
	if err != nil {
		t.Fatal(err)
	}
	resp, err := server.GetProviderSchema(ctx, &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range resp.Diagnostics {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			t.Errorf("GetProviderSchema: %s: %s", d.Summary, d.Detail)
		}
	}
	for _, name := range []string{"defectdojo_engagement", "defectdojo_product", "defectdojo_product_type"} {
		if _, ok := resp.ResourceSchemas[name]; !ok {
			t.Errorf("resource %s is not registered", name)
		}
		if _, ok := resp.DataSourceSchemas[name]; !ok {
			t.Errorf("data source %s is not registered", name)
		}
	}
}

func TestBaseURLValidator(t *testing.T) {
	tests := map[string]bool{
		"https://defectdojo.example.com":     true,
		"HTTPS://defectdojo.example.com":     true, // the client accepts it
		"http://localhost:8080/defectdojo/":  true,
		"":                                   true, // reported as missing
		"defectdojo.example.com":             false,
		"ftp://defectdojo.example.com":       false,
		"https://":                           false,
		"https://defectdojo.example.com/%zz": false,
		"/api/v2/":                           false,
	}
	for in, valid := range tests {
		var resp validator.StringResponse
		baseURLValidator{}.ValidateString(context.Background(), validator.StringRequest{
			Path: path.Root("url"), ConfigValue: types.StringValue(in),
		}, &resp)
		if resp.Diagnostics.HasError() == valid {
			t.Errorf("%q: valid = %t, want %t (%v)", in, !resp.Diagnostics.HasError(), valid, resp.Diagnostics)
		}
	}
}
