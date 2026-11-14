package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// fakeProductTypes is a minimal DefectDojo that serves one product type and
// records the bodies of the PATCH requests, to check what the provider sends.
type fakeProductTypes struct {
	mu      sync.Mutex
	object  map[string]any // nil when deleted
	patches []map[string]any
}

func (f *fakeProductTypes) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, status int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if err := json.NewEncoder(w).Encode(v); err != nil {
			t.Error(err)
		}
	}
	decode := func(r *http.Request) map[string]any {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decoding %s %s: %v", r.Method, r.URL, err)
		}
		return body
	}
	mux.HandleFunc("POST /api/v2/product_types/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.object = map[string]any{"id": 7, "description": nil, "critical_product": false, "key_product": false, "authorized_users": []any{}}
		for k, v := range decode(r) {
			f.object[k] = v
		}
		write(w, http.StatusCreated, f.object)
	})
	object := func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.object == nil {
			write(w, http.StatusNotFound, map[string]string{"detail": "No Product_Type matches the given query."})
			return
		}
		switch r.Method {
		case http.MethodGet:
		case http.MethodPatch:
			body := decode(r)
			f.patches = append(f.patches, body)
			for k, v := range body {
				f.object[k] = v
			}
		case http.MethodDelete:
			f.object = nil
			w.WriteHeader(http.StatusNoContent)
			return
		}
		write(w, http.StatusOK, f.object)
	}
	for _, method := range []string{"GET", "PATCH", "DELETE"} {
		mux.HandleFunc(method+" /api/v2/product_types/7/", object)
	}
	mux.HandleFunc("GET /api/v2/products/", func(w http.ResponseWriter, _ *http.Request) {
		write(w, http.StatusOK, map[string]any{"count": 0, "results": []any{}})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected %s %s", r.Method, r.URL)
		http.NotFound(w, r)
	})
	return mux
}

// setAuthorizedUsers changes the authorized users outside Terraform.
func (f *fakeProductTypes) setAuthorizedUsers(ids ...int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	users := make([]any, len(ids))
	for i, id := range ids {
		users[i] = id
	}
	f.object["authorized_users"] = users
}

// lastPatch returns the body of the last PATCH request.
func (f *fakeProductTypes) lastPatch() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.patches) == 0 {
		return nil
	}
	return f.patches[len(f.patches)-1]
}

// Updates send authorized_users only when it is configured, so that users
// managed elsewhere are kept. Runs against a fake DefectDojo, to check the
// requests.
func TestAccProductType_authorizedUsersNotSentUnlessConfigured(t *testing.T) {
	fake := &fakeProductTypes{}
	server := httptest.NewServer(fake.handler(t))
	t.Cleanup(server.Close)

	config := func(description, extra string) string {
		return fmt.Sprintf(`
provider "defectdojo" {
  url       = %[1]q
  api_token = "test"
}

resource "defectdojo_product_type" "test" {
  name        = "fake"
  description = %[2]q
%[3]s}
`, server.URL, description, extra)
	}
	checkPatch := func(wantUsers bool) resource.TestCheckFunc {
		return func(*terraform.State) error {
			patch := fake.lastPatch()
			if patch == nil {
				return fmt.Errorf("no PATCH request was sent")
			}
			if _, sent := patch["authorized_users"]; sent != wantUsers {
				return fmt.Errorf("PATCH body %v: authorized_users sent = %v, want %v", patch, sent, wantUsers)
			}
			return nil
		}
	}

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: config("one", "  authorized_users = [1]\n"),
			},
			// Configured: sent.
			{
				Config: config("two", "  authorized_users = [1, 2]\n"),
				Check:  checkPatch(true),
			},
			// Not configured: not sent, and a change made elsewhere is kept.
			{
				PreConfig: func() { fake.setAuthorizedUsers(1, 2, 3) },
				Config:    config("three", ""),
				Check:     checkPatch(false),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(productTypeAddress, tfjsonpath.New("authorized_users"), knownvalue.SetSizeExact(3)),
					statecheck.ExpectKnownValue(productTypeAddress, tfjsonpath.New("id"), knownvalue.Int64Exact(7)),
				},
			},
			// Configured as empty: sent, and removes them all.
			{
				Config: config("four", "  authorized_users = []\n"),
				Check:  checkPatch(true),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(productTypeAddress, tfjsonpath.New("authorized_users"), knownvalue.SetSizeExact(0)),
				},
			},
		},
	})
}
