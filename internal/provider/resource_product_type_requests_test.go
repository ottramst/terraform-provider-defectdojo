package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
)

// fakeDefectDojo is a minimal DefectDojo that serves one object of a
// collection, such as "product_types", and records the bodies of the POST and
// PATCH requests, to check what the provider sends. With singleton, it serves
// an object that always exists, like the system settings: object is set
// before the test, and a GET by ID, a POST and a DELETE are 405, as in
// DefectDojo. A list of the collection
// returns the object when it matches the query's filters. Lists of the
// collections in lists return their objects, filtered the same way; other GET
// requests to /api/v2/ (the child counts of force_destroy) return an empty
// list.
type fakeDefectDojo struct {
	collection string
	defaults   map[string]any // the fields of a new object
	id         int            // the object's ID; 7 when zero
	singleton  bool

	// lists holds read-only objects of other collections, by collection.
	lists map[string][]map[string]any

	// forbidden lists collections whose lists answer 403, as superuser-only
	// endpoints do for other users.
	forbidden map[string]bool

	mu      sync.Mutex
	object  map[string]any // nil when deleted
	posts   []map[string]any
	patches []map[string]any
	writes  int // the number of POST, PATCH and DELETE requests
}

func (f *fakeDefectDojo) objectID() int {
	if f.id == 0 {
		return 7
	}
	return f.id
}

// matches reports whether object matches the filters of a list query: each
// parameter other than limit and offset equals the field, formatted with
// fmt.Sprint ("" for null).
func matches(object map[string]any, query url.Values) bool {
	for k, values := range query {
		if k == "limit" || k == "offset" {
			continue
		}
		v := object[k]
		if v == nil {
			v = ""
		}
		if fmt.Sprint(v) != values[0] {
			return false
		}
	}
	return true
}

func (f *fakeDefectDojo) handler(t *testing.T) http.Handler {
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
	notAllowed := func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Method != http.MethodGet {
			f.writes++
		}
		write(w, http.StatusMethodNotAllowed, map[string]string{"detail": fmt.Sprintf("Method %q not allowed.", r.Method)})
	}
	objectPath := fmt.Sprintf("/api/v2/%s/%d/", f.collection, f.objectID())
	if f.singleton {
		mux.HandleFunc("POST /api/v2/"+f.collection+"/", notAllowed)
		mux.HandleFunc("GET "+objectPath, notAllowed)
		mux.HandleFunc("DELETE "+objectPath, notAllowed)
	}
	post := func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.writes++
		f.object = map[string]any{"id": f.objectID()}
		for k, v := range f.defaults {
			f.object[k] = v
		}
		body := decode(r)
		f.posts = append(f.posts, body)
		for k, v := range body {
			if v != nil || f.object[k] == nil {
				f.object[k] = v
			}
		}
		write(w, http.StatusCreated, f.object)
	}
	if !f.singleton {
		mux.HandleFunc("POST /api/v2/"+f.collection+"/", post)
	}
	object := func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.object == nil {
			write(w, http.StatusNotFound, map[string]string{"detail": "No object matches the given query."})
			return
		}
		if r.Method != http.MethodGet {
			f.writes++
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
		if !f.singleton || method == "PATCH" {
			mux.HandleFunc(method+" "+objectPath, object)
		}
	}
	mux.HandleFunc("GET /api/v2/", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		collection := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v2/"), "/")
		if f.forbidden[collection] {
			write(w, http.StatusForbidden, map[string]string{"detail": "You do not have permission to perform this action."})
			return
		}
		objects := f.lists[collection]
		if r.URL.Path == "/api/v2/"+f.collection+"/" && f.object != nil {
			objects = []map[string]any{f.object}
		}
		results := []map[string]any{}
		for _, o := range objects {
			if matches(o, r.URL.Query()) {
				results = append(results, o)
			}
		}
		write(w, http.StatusOK, map[string]any{"count": len(results), "results": results})
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected %s %s", r.Method, r.URL)
		http.NotFound(w, r)
	})
	return mux
}

// setList replaces the read-only objects of another collection.
func (f *fakeDefectDojo) setList(collection string, objects []map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.lists == nil {
		f.lists = make(map[string][]map[string]any)
	}
	f.lists[collection] = objects
}

// set changes a field of the object outside Terraform.
func (f *fakeDefectDojo) set(field string, value any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.object[field] = value
}

// lastPost returns the body of the last POST request.
func (f *fakeDefectDojo) lastPost() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.posts) == 0 {
		return nil
	}
	return f.posts[len(f.posts)-1]
}

// lastPatch returns the body of the last PATCH request.
func (f *fakeDefectDojo) lastPatch() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.patches) == 0 {
		return nil
	}
	return f.patches[len(f.patches)-1]
}

// checkPatchSends checks which of fields the last PATCH request sent.
func (f *fakeDefectDojo) checkPatchSends(want bool, fields ...string) resource.TestCheckFunc {
	return checkSends("PATCH", f.lastPatch, want, fields)
}

// checkPostSends checks which of fields the last POST request sent.
func (f *fakeDefectDojo) checkPostSends(want bool, fields ...string) resource.TestCheckFunc {
	return checkSends("POST", f.lastPost, want, fields)
}

func checkSends(method string, last func() map[string]any, want bool, fields []string) resource.TestCheckFunc {
	return func(*terraform.State) error {
		body := last()
		if body == nil {
			return fmt.Errorf("no %s request was sent", method)
		}
		for _, field := range fields {
			if _, sent := body[field]; sent != want {
				return fmt.Errorf("%s body %v: %s sent = %v, want %v", method, body, field, sent, want)
			}
		}
		return nil
	}
}

// Updates send authorized_users only when it is configured, so that users
// managed elsewhere are kept. Runs against a fake DefectDojo, to check the
// requests.
func TestAccProductType_authorizedUsersNotSentUnlessConfigured(t *testing.T) {
	fake := &fakeDefectDojo{
		collection: "product_types",
		defaults:   map[string]any{"description": nil, "critical_product": false, "key_product": false, "authorized_users": []any{}},
	}
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
		return fake.checkPatchSends(wantUsers, "authorized_users")
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
				PreConfig: func() { fake.set("authorized_users", []any{1, 2, 3}) },
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

// writeCount returns the number of POST, PATCH and DELETE requests.
func (f *fakeDefectDojo) writeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.writes
}
