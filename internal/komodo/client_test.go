package komodo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const fixtureAPIKey = "fixture-api-key"
const fixtureAPISecret = "fixture-api-secret"

func newFixtureClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	client, err := newClientWithTransport(ClientConfig{
		Endpoint:  server.URL,
		APIKey:    fixtureAPIKey,
		APISecret: fixtureAPISecret,
		Timeout:   time.Second,
	}, server.Client().Transport)
	if err != nil {
		t.Fatalf("create fixture client: %v", err)
	}
	return client, server
}

func writeFixtureJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func inventoryFixtureHandler(t *testing.T, override func(http.ResponseWriter, *http.Request) bool) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("request method = %s, want POST", r.Method)
			writeFixtureJSON(w, http.StatusMethodNotAllowed, `{}`)
			return
		}
		if r.Header.Get("X-Api-Key") != fixtureAPIKey || r.Header.Get("X-Api-Secret") != fixtureAPISecret {
			t.Errorf("Komodo API credentials were not sent in their documented headers")
			writeFixtureJSON(w, http.StatusUnauthorized, `{}`)
			return
		}
		if override != nil && override(w, r) {
			return
		}
		switch r.URL.Path {
		case "/read/GetVersion":
			writeFixtureJSON(w, http.StatusOK, `{"version":"2.3.4"}`)
		case "/read/ListPermissions":
			writeFixtureJSON(w, http.StatusOK, `[
				{"resource_target":{"type":"Server","id":"all"},"level":"Read","specific":[]},
				{"resource_target":{"type":"Stack","id":"all"},"level":"Read","specific":[]},
				{"resource_target":{"type":"Deployment","id":"all"},"level":"Read","specific":[]},
				{"resource_target":{"type":"Swarm","id":"all"},"level":"Read","specific":[]}
			]`)
		case "/read/GetPermission":
			writeFixtureJSON(w, http.StatusOK, `{"level":"Read","specific":[]}`)
		case "/read/ListServers", "/read/ListStacks", "/read/ListDeployments", "/read/ListSwarms":
			writeFixtureJSON(w, http.StatusOK, `[]`)
		default:
			t.Errorf("unexpected Komodo API method %q", r.URL.Path)
			writeFixtureJSON(w, http.StatusNotFound, `{}`)
		}
	}
}

func TestClientAuthenticatesAgainstCoreV2AndAcceptsEmptyScopedInventory(t *testing.T) {
	client, _ := newFixtureClient(t, inventoryFixtureHandler(t, nil))
	inventory, err := client.CollectInventory(context.Background())
	if err != nil {
		t.Fatalf("CollectInventory: %v", err)
	}
	if inventory.Version != "2.3.4" {
		t.Fatalf("version = %q, want Core version from GetVersion", inventory.Version)
	}
	if inventory.Count() != 0 {
		t.Fatalf("inventory count = %d, want a successful empty inventory", inventory.Count())
	}
	if len(inventory.Scope.Verified) != 4 {
		t.Fatalf("verified scope = %v, want all four configured resource kinds", inventory.Scope.Verified)
	}
	if len(inventory.Scope.SpecificPermissionsVerified) != 0 || len(inventory.Scope.SpecificPermissionsUnverified) != 4 {
		t.Fatalf("specific permission scope = verified %v, unverified %v; want all empty inventories left unverified despite direct grants", inventory.Scope.SpecificPermissionsVerified, inventory.Scope.SpecificPermissionsUnverified)
	}
}

func TestClientRejectsInvalidAuthenticationWithoutEchoingResponse(t *testing.T) {
	secret := fixtureAPISecret
	client, _ := newFixtureClient(t, inventoryFixtureHandler(t, func(w http.ResponseWriter, r *http.Request) bool {
		writeFixtureJSON(w, http.StatusUnauthorized, `{"error":"bad `+secret+`"}`)
		return true
	}))
	_, err := client.CollectInventory(context.Background())
	if err == nil {
		t.Fatal("expected authentication error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "auth") {
		t.Fatalf("error = %q, want safe authentication classification", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaked API secret: %q", err)
	}
}

func TestClientIdentifiesMalformedReadContractWithoutEchoingResponse(t *testing.T) {
	client, _ := newFixtureClient(t, inventoryFixtureHandler(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/read/ListPermissions" {
			writeFixtureJSON(w, http.StatusOK, `"`+fixtureAPISecret+`"`)
			return true
		}
		return false
	}))
	_, err := client.CollectInventory(context.Background())
	if err == nil || !strings.Contains(err.Error(), "ListPermissions") || !strings.Contains(err.Error(), "top-level") || !strings.Contains(err.Error(), "received string") {
		t.Fatalf("CollectInventory error = %v, want safe failing-method context", err)
	}
	if strings.Contains(err.Error(), fixtureAPISecret) {
		t.Fatalf("error leaked response material: %q", err)
	}
}

func TestClientRejectsUnsupportedAPIVersionBeforeListingResources(t *testing.T) {
	methods := []string{}
	client, _ := newFixtureClient(t, inventoryFixtureHandler(t, func(w http.ResponseWriter, r *http.Request) bool {
		methods = append(methods, r.URL.Path)
		if r.URL.Path == "/read/GetVersion" {
			writeFixtureJSON(w, http.StatusOK, `{"version":"1.9.0"}`)
			return true
		}
		return false
	}))
	_, err := client.CollectInventory(context.Background())
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "version") {
		t.Fatalf("CollectInventory error = %v, want unsupported version", err)
	}
	if len(methods) != 1 || methods[0] != "/read/GetVersion" {
		t.Fatalf("methods = %v, want only GetVersion before version rejection", methods)
	}
}

func TestClientPaginatesAllFourResourceKinds(t *testing.T) {
	client, _ := newFixtureClient(t, inventoryFixtureHandler(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != "/read/ListServers" && r.URL.Path != "/read/ListStacks" && r.URL.Path != "/read/ListDeployments" && r.URL.Path != "/read/ListSwarms" {
			return false
		}
		var request struct {
			Page  int `json:"page"`
			Limit int `json:"limit"`
		}
		if err := jsonNewDecoder(r).Decode(&request); err != nil {
			t.Errorf("decode page request: %v", err)
			writeFixtureJSON(w, http.StatusBadRequest, `{}`)
			return true
		}
		if request.Limit != 2 {
			t.Errorf("page limit = %d, want bounded fixture limit 2", request.Limit)
		}
		if request.Page == 0 {
			writeFixtureJSON(w, http.StatusOK, fixtureItemsForPath(r.URL.Path, "one", "two"))
		} else {
			writeFixtureJSON(w, http.StatusOK, fixtureItemsForPath(r.URL.Path, "three"))
		}
		return true
	}))
	client.pageSize = 2
	inventory, err := client.CollectInventory(context.Background())
	if err != nil {
		t.Fatalf("CollectInventory: %v", err)
	}
	if inventory.Count() != 12 {
		t.Fatalf("inventory count = %d, want 3 items for each of four kinds", inventory.Count())
	}
}

func TestClientRejectsDuplicateNativeIDsAcrossPages(t *testing.T) {
	client, _ := newFixtureClient(t, inventoryFixtureHandler(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/read/ListServers" {
			var request struct {
				Page int `json:"page"`
			}
			_ = jsonNewDecoder(r).Decode(&request)
			if request.Page == 0 {
				writeFixtureJSON(w, http.StatusOK, fixtureItemsForPath(r.URL.Path, "same", "first"))
			} else {
				writeFixtureJSON(w, http.StatusOK, fixtureItemsForPath(r.URL.Path, "same"))
			}
			return true
		}
		return false
	}))
	client.pageSize = 2
	_, err := client.CollectInventory(context.Background())
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "duplicate") {
		t.Fatalf("CollectInventory error = %v, want duplicate-ID rejection", err)
	}
}

func TestClientReportsEmptyKindsWithoutDirectGrantAsSpecificScopeUnverified(t *testing.T) {
	client, _ := newFixtureClient(t, inventoryFixtureHandler(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/read/ListPermissions" {
			writeFixtureJSON(w, http.StatusOK, `[]`)
			return true
		}
		return false
	}))
	inventory, err := client.CollectInventory(context.Background())
	if err != nil {
		t.Fatalf("CollectInventory: successful empty list methods should remain usable: %v", err)
	}
	if inventory.Count() != 0 || len(inventory.Scope.Verified) != len(resourceKinds) {
		t.Fatalf("empty inventory=%d verified scope=%v, want empty inventory and all four read methods verified", inventory.Count(), inventory.Scope.Verified)
	}
	if len(inventory.Scope.SpecificPermissionsVerified) != 0 || len(inventory.Scope.SpecificPermissionsUnverified) != len(resourceKinds) {
		t.Fatalf("specific permission scope = verified %v, unverified %v; want all kinds unverified", inventory.Scope.SpecificPermissionsVerified, inventory.Scope.SpecificPermissionsUnverified)
	}
}

func TestClientRejectsCredentialWithElevatedDirectPermissions(t *testing.T) {
	methods := []string{}
	client, _ := newFixtureClient(t, inventoryFixtureHandler(t, func(w http.ResponseWriter, r *http.Request) bool {
		methods = append(methods, r.URL.Path)
		if r.URL.Path == "/read/ListPermissions" {
			writeFixtureJSON(w, http.StatusOK, `[{"resource_target":{"type":"Deployment","id":"all"},"level":"Execute"}]`)
			return true
		}
		return false
	}))
	_, err := client.CollectInventory(context.Background())
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "read-only") {
		t.Fatalf("CollectInventory error = %v, want elevated-permission rejection", err)
	}
	if containsString(methods, "/read/ListServers") {
		t.Fatalf("inventory was listed after elevated access was found: %v", methods)
	}
}

func TestClientRejectsSpecificPermissionsBeyondInventoryRead(t *testing.T) {
	t.Run("direct grant", func(t *testing.T) {
		methods := []string{}
		client, _ := newFixtureClient(t, inventoryFixtureHandler(t, func(w http.ResponseWriter, r *http.Request) bool {
			methods = append(methods, r.URL.Path)
			if r.URL.Path == "/read/ListPermissions" {
				writeFixtureJSON(w, http.StatusOK, `[{"resource_target":{"type":"Server","id":"all"},"level":"Read","specific":["Logs"]}]`)
				return true
			}
			return false
		}))
		_, err := client.CollectInventory(context.Background())
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "read-only") {
			t.Fatalf("CollectInventory error = %v, want rejection of specific permissions", err)
		}
		if containsString(methods, "/read/ListServers") {
			t.Fatalf("inventory was listed after a specific permission was found: %v", methods)
		}
	})

	t.Run("effective item grant", func(t *testing.T) {
		client, _ := newFixtureClient(t, inventoryFixtureHandler(t, func(w http.ResponseWriter, r *http.Request) bool {
			switch r.URL.Path {
			case "/read/ListServers":
				writeFixtureJSON(w, http.StatusOK, fixtureItemsForPath(r.URL.Path, "server-1"))
				return true
			case "/read/GetPermission":
				writeFixtureJSON(w, http.StatusOK, `{"level":"Read","specific":["Inspect"]}`)
				return true
			}
			return false
		}))
		_, err := client.CollectInventory(context.Background())
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "scope") {
			t.Fatalf("CollectInventory error = %v, want rejection of effective specific permission", err)
		}
	})
}

func TestClientUsesOnlyAllowlistedReadMethods(t *testing.T) {
	methods := []string{}
	client, _ := newFixtureClient(t, inventoryFixtureHandler(t, func(w http.ResponseWriter, r *http.Request) bool {
		methods = append(methods, r.URL.Path)
		return false
	}))
	if err := client.call(context.Background(), "ExecuteDeployment", map[string]string{"deployment": "x"}, nil); err == nil {
		t.Fatal("unexpected method was accepted")
	}
	if len(methods) != 0 {
		t.Fatalf("unexpected method sent to Komodo: %v", methods)
	}
}

func TestClientRedactsTransientErrorsAndHonorsTimeout(t *testing.T) {
	client, _ := newFixtureClient(t, inventoryFixtureHandler(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/read/ListStacks" {
			writeFixtureJSON(w, http.StatusServiceUnavailable, `{"detail":"`+fixtureAPISecret+`"}`)
			return true
		}
		return false
	}))
	_, err := client.CollectInventory(context.Background())
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "upstream") {
		t.Fatalf("CollectInventory error = %v, want transient upstream classification", err)
	}
	if strings.Contains(err.Error(), fixtureAPISecret) {
		t.Fatalf("error leaked secret response data: %q", err)
	}

	timeoutClient, _ := newFixtureClient(t, inventoryFixtureHandler(t, func(w http.ResponseWriter, r *http.Request) bool {
		time.Sleep(60 * time.Millisecond)
		writeFixtureJSON(w, http.StatusOK, `{"version":"2.3.4"}`)
		return true
	}))
	timeoutClient.httpClient.Timeout = 10 * time.Millisecond
	_, err = timeoutClient.CollectInventory(context.Background())
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "timeout") {
		t.Fatalf("timeout error = %v, want timeout classification", err)
	}
}

func TestClientChecksPermissionsOnVisibleResources(t *testing.T) {
	methods := []string{}
	client, _ := newFixtureClient(t, inventoryFixtureHandler(t, func(w http.ResponseWriter, r *http.Request) bool {
		methods = append(methods, r.URL.Path)
		switch r.URL.Path {
		case "/read/ListServers":
			writeFixtureJSON(w, http.StatusOK, fixtureItemsForPath(r.URL.Path, "server-1"))
			return true
		case "/read/GetPermission":
			writeFixtureJSON(w, http.StatusOK, `{"level":"Read","specific":[]}`)
			return true
		}
		return false
	}))
	_, err := client.CollectInventory(context.Background())
	if err != nil {
		t.Fatalf("CollectInventory: %v", err)
	}
	if !containsString(methods, "/read/GetPermission") {
		t.Fatalf("methods = %v, want GetPermission check for visible Server", methods)
	}
}

func TestClientRecognizesLevelOnlyReadWithoutClaimingSpecificPermissionsAreAbsent(t *testing.T) {
	client, _ := newFixtureClient(t, inventoryFixtureHandler(t, func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/read/ListServers" {
			writeFixtureJSON(w, http.StatusOK, fixtureItemsForPath(r.URL.Path, "server-1"))
			return true
		}
		if r.URL.Path == "/read/GetPermission" {
			writeFixtureJSON(w, http.StatusOK, `"Read"`)
			return true
		}
		return false
	}))
	inventory, err := client.CollectInventory(context.Background())
	if err != nil {
		t.Fatalf("level-only Read should authorize this client's read-only collection methods: %v", err)
	}
	if len(inventory.Servers) != 1 || !containsResourceKind(inventory.Scope.SpecificPermissionsUnverified, KindServer) {
		t.Fatalf("inventory=%+v, want one server and specific permission state explicitly unverified", inventory)
	}
	if containsResourceKind(inventory.Scope.SpecificPermissionsVerified, KindServer) {
		t.Fatal("level-only Read was treated as proof that effective specific permissions are absent")
	}
}

func TestClientRejectsMalformedStructuredGetPermissionResponses(t *testing.T) {
	for name, response := range map[string]string{
		"missing level":    `{"specific":[]}`,
		"missing specific": `{"level":"Read"}`,
		"null specific":    `{"level":"Read","specific":null}`,
		"wrong level type": `{"level":7,"specific":[]}`,
		"wrong specific":   `{"level":"Read","specific":"Logs"}`,
		"unexpected field": `{"level":"Read","specific":[],"qualificationSecret":"must-not-leak"}`,
		"duplicate field":  `{"level":"Write","level":"Read","specific":[]}`,
		"null response":    `null`,
		"array response":   `[]`,
		"number response":  `42`,
	} {
		t.Run(name, func(t *testing.T) {
			client, _ := newFixtureClient(t, inventoryFixtureHandler(t, func(w http.ResponseWriter, r *http.Request) bool {
				if r.URL.Path == "/read/ListServers" {
					writeFixtureJSON(w, http.StatusOK, fixtureItemsForPath(r.URL.Path, "server-1"))
					return true
				}
				if r.URL.Path == "/read/GetPermission" {
					writeFixtureJSON(w, http.StatusOK, response)
					return true
				}
				return false
			}))
			_, err := client.CollectInventory(context.Background())
			if err == nil {
				t.Fatal("CollectInventory unexpectedly accepted a malformed GetPermission response")
			}
			if strings.Contains(err.Error(), "must-not-leak") {
				t.Fatalf("error leaked response material: %v", err)
			}
		})
	}
}

func TestClientChecksEffectivePermissionsForEveryListedResource(t *testing.T) {
	client, _ := newFixtureClient(t, inventoryFixtureHandler(t, func(w http.ResponseWriter, r *http.Request) bool {
		switch r.URL.Path {
		case "/read/ListServers":
			writeFixtureJSON(w, http.StatusOK, fixtureItemsForPath(r.URL.Path, "server-1", "server-2"))
			return true
		case "/read/GetPermission":
			var request struct {
				Target struct {
					ID string `json:"id"`
				} `json:"target"`
			}
			if err := jsonNewDecoder(r).Decode(&request); err != nil {
				t.Errorf("decode permission target: %v", err)
				writeFixtureJSON(w, http.StatusBadRequest, `{}`)
				return true
			}
			if request.Target.ID == "server-2" {
				writeFixtureJSON(w, http.StatusOK, `{"level":"Read","specific":["Terminal"]}`)
			} else {
				writeFixtureJSON(w, http.StatusOK, `{"level":"Read","specific":[]}`)
			}
			return true
		}
		return false
	}))
	_, err := client.CollectInventory(context.Background())
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "specific permissions") {
		t.Fatalf("CollectInventory error = %v, want rejection of elevated effective permissions on the second resource", err)
	}
}

func TestClientRejectsUnsafeLevelOnlyGetPermissionResponses(t *testing.T) {
	for _, level := range []string{"None", "Execute", "Write", "Unknown"} {
		t.Run(level, func(t *testing.T) {
			client, _ := newFixtureClient(t, inventoryFixtureHandler(t, func(w http.ResponseWriter, r *http.Request) bool {
				if r.URL.Path == "/read/ListServers" {
					writeFixtureJSON(w, http.StatusOK, fixtureItemsForPath(r.URL.Path, "server-1"))
					return true
				}
				if r.URL.Path == "/read/GetPermission" {
					body, _ := json.Marshal(level)
					writeFixtureJSON(w, http.StatusOK, string(body))
					return true
				}
				return false
			}))
			_, err := client.CollectInventory(context.Background())
			if err == nil || strings.Contains(strings.ToLower(err.Error()), "specific permissions") {
				t.Fatalf("CollectInventory error = %v, want rejection due to an unsafe or unknown permission level", err)
			}
			if strings.Contains(err.Error(), "incompatible JSON type") {
				t.Fatalf("level-only response was not recognized: %v", err)
			}
		})
	}
}

func TestClientRejectsUnsafeStructuredGetPermissionLevels(t *testing.T) {
	for _, level := range []string{"None", "Execute", "Write", "Unknown"} {
		t.Run(level, func(t *testing.T) {
			client, _ := newFixtureClient(t, inventoryFixtureHandler(t, func(w http.ResponseWriter, r *http.Request) bool {
				if r.URL.Path == "/read/ListServers" {
					writeFixtureJSON(w, http.StatusOK, fixtureItemsForPath(r.URL.Path, "server-1"))
					return true
				}
				if r.URL.Path == "/read/GetPermission" {
					writeFixtureJSON(w, http.StatusOK, fmt.Sprintf(`{"level":%q,"specific":[]}`, level))
					return true
				}
				return false
			}))
			_, err := client.CollectInventory(context.Background())
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), "scope") {
				t.Fatalf("CollectInventory error = %v, want rejection of unsafe or unknown level", err)
			}
		})
	}
}

func fixtureItemsForPath(path string, ids ...string) string {
	items := make([]string, 0, len(ids))
	resourceType := strings.TrimPrefix(path, "/read/List")
	resourceType = strings.TrimSuffix(resourceType, "s")
	for _, id := range ids {
		state := "Running"
		info := fmt.Sprintf(`{"state":%q}`, state)
		switch path {
		case "/read/ListServers":
			info = `{"state":"Ok","err":{"message":"do not persist this"}}`
		case "/read/ListStacks":
			info = `{"state":"Running","server_id":"server-1","swarm_id":""}`
		case "/read/ListDeployments":
			info = `{"state":"Running","server_id":"server-1","swarm_id":""}`
		case "/read/ListSwarms":
			info = `{"state":"Healthy","server_ids":["server-1"]}`
		}
		items = append(items, fmt.Sprintf(`{"id":%q,"type":%q,"name":%q,"template":false,"tags":[],"info":%s}`, id, resourceType, "same-name", info))
	}
	return "[" + strings.Join(items, ",") + "]"
}

func containsResourceKind(kinds []ResourceKind, target ResourceKind) bool {
	for _, kind := range kinds {
		if kind == target {
			return true
		}
	}
	return false
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func jsonNewDecoder(r *http.Request) *json.Decoder { return json.NewDecoder(r.Body) }

func TestPermissionLevelOrder(t *testing.T) {
	for _, level := range []string{"Read", "read"} {
		if !permissionSatisfiesRead(level) {
			t.Errorf("permission %q should satisfy Komodo inventory read", level)
		}
	}
	for _, level := range []string{"", "None", "Logs", "Execute", "Write"} {
		if permissionSatisfiesRead(level) {
			t.Errorf("permission %q should not satisfy minimum read-only scope", level)
		}
	}
}
