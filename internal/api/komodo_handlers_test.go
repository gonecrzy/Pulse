package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rcourtman/pulse-go-rewrite/internal/config"
	"github.com/rcourtman/pulse-go-rewrite/internal/komodo"
)

type testKomodoClient struct{}

func (testKomodoClient) CollectInventory(context.Context) (komodo.Inventory, error) {
	return komodo.Inventory{Version: "2.3.4"}, nil
}

type testKomodoErrorClient struct{ err error }

func (c testKomodoErrorClient) CollectInventory(context.Context) (komodo.Inventory, error) {
	return komodo.Inventory{}, c.err
}

func TestKomodoConnectionCRUDRedactsAndPreservesCredentialRotation(t *testing.T) {
	persistence := config.NewConfigPersistence(t.TempDir())
	handler := &KomodoHandlers{
		getPersistence: func(context.Context) *config.ConfigPersistence { return persistence },
		newClient:      func(config.KomodoInstance) (komodoInventoryClient, error) { return testKomodoClient{}, nil },
	}

	requestBody := `{"name":"lab","endpoint":"https://komodo.example.test","apiKey":"key-1","apiSecret":"secret-1","enabled":true}`
	addRequest := httptest.NewRequest(http.MethodPost, "/api/komodo/connections", strings.NewReader(requestBody))
	addResponse := httptest.NewRecorder()
	handler.HandleAdd(addResponse, addRequest)
	if addResponse.Code != http.StatusCreated {
		t.Fatalf("add status = %d, body=%s", addResponse.Code, addResponse.Body.String())
	}
	var redacted config.KomodoInstance
	if err := json.NewDecoder(addResponse.Body).Decode(&redacted); err != nil {
		t.Fatalf("decode add response: %v", err)
	}
	if redacted.APIKey != "********" || redacted.APISecret != "********" {
		t.Fatalf("add response did not mask credentials: %+v", redacted)
	}
	if strings.Contains(addResponse.Body.String(), "key-1") || strings.Contains(addResponse.Body.String(), "secret-1") {
		t.Fatalf("add response leaked credentials: %s", addResponse.Body.String())
	}

	loaded, err := persistence.LoadKomodoConfig()
	if err != nil || len(loaded) != 1 || loaded[0].APIKey != "key-1" || loaded[0].APISecret != "secret-1" {
		t.Fatalf("encrypted config round trip = %+v, err=%v", loaded, err)
	}

	updateBody := `{"name":"lab-updated","endpoint":"https://komodo.example.test","apiKey":"********","apiSecret":"secret-2","enabled":false}`
	updateRequest := httptest.NewRequest(http.MethodPut, "/api/komodo/connections/"+redacted.ID, strings.NewReader(updateBody))
	updateResponse := httptest.NewRecorder()
	handler.HandleUpdate(updateResponse, updateRequest)
	if updateResponse.Code != http.StatusOK {
		t.Fatalf("update status = %d, body=%s", updateResponse.Code, updateResponse.Body.String())
	}
	loaded, err = persistence.LoadKomodoConfig()
	if err != nil || len(loaded) != 1 || loaded[0].APIKey != "key-1" || loaded[0].APISecret != "secret-2" || loaded[0].Enabled {
		t.Fatalf("updated config = %+v, err=%v", loaded, err)
	}

	listRequest := httptest.NewRequest(http.MethodGet, "/api/komodo/connections", nil)
	listResponse := httptest.NewRecorder()
	handler.HandleList(listResponse, listRequest)
	if listResponse.Code != http.StatusOK || strings.Contains(listResponse.Body.String(), "key-1") || strings.Contains(listResponse.Body.String(), "secret-2") {
		t.Fatalf("list response was not safely redacted: status=%d body=%s", listResponse.Code, listResponse.Body.String())
	}
}

func TestKomodoSavedTestUsesCandidateWithoutSavingOrLeakingCredentials(t *testing.T) {
	persistence := config.NewConfigPersistence(t.TempDir())
	instance := config.KomodoInstance{
		ID:        "87ef53c3-9a5a-4d7b-a159-997ccca19811",
		Name:      "lab",
		Endpoint:  "https://komodo.example.test",
		APIKey:    "stored-key",
		APISecret: "stored-secret",
		Enabled:   true,
	}
	if err := persistence.SaveKomodoConfig([]config.KomodoInstance{instance}); err != nil {
		t.Fatalf("save fixture config: %v", err)
	}
	var tested config.KomodoInstance
	handler := &KomodoHandlers{
		getPersistence: func(context.Context) *config.ConfigPersistence { return persistence },
		newClient: func(candidate config.KomodoInstance) (komodoInventoryClient, error) {
			tested = candidate
			return testKomodoClient{}, nil
		},
	}
	candidateBody := `{"name":"lab","endpoint":"https://komodo.example.test","apiKey":"********","apiSecret":"rotated-secret","enabled":true}`
	request := httptest.NewRequest(http.MethodPost, "/api/komodo/connections/"+instance.ID+"/test", strings.NewReader(candidateBody))
	response := httptest.NewRecorder()
	handler.HandleTestSavedConnection(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("test status = %d, body=%s", response.Code, response.Body.String())
	}
	if tested.APIKey != "stored-key" || tested.APISecret != "rotated-secret" {
		t.Fatalf("test used unexpected credentials: %+v", tested)
	}
	if strings.Contains(response.Body.String(), "stored-key") || strings.Contains(response.Body.String(), "rotated-secret") {
		t.Fatalf("test response leaked credentials: %s", response.Body.String())
	}
	loaded, err := persistence.LoadKomodoConfig()
	if err != nil || len(loaded) != 1 || loaded[0].APISecret != "stored-secret" {
		t.Fatalf("candidate test modified saved config: %+v, err=%v", loaded, err)
	}
}

func TestKomodoFailedCredentialRotationKeepsStoredCredentials(t *testing.T) {
	persistence := config.NewConfigPersistence(t.TempDir())
	instance := config.KomodoInstance{
		ID:        "87ef53c3-9a5a-4d7b-a159-997ccca19811",
		Name:      "lab",
		Endpoint:  "https://komodo.example.test",
		APIKey:    "stored-key",
		APISecret: "stored-secret",
		Enabled:   true,
	}
	if err := persistence.SaveKomodoConfig([]config.KomodoInstance{instance}); err != nil {
		t.Fatalf("save fixture config: %v", err)
	}
	handler := &KomodoHandlers{
		getPersistence: func(context.Context) *config.ConfigPersistence { return persistence },
		newClient: func(config.KomodoInstance) (komodoInventoryClient, error) {
			return testKomodoErrorClient{err: errors.New("candidate-secret appeared in an upstream failure")}, nil
		},
	}
	updateBody := `{"name":"lab","endpoint":"https://komodo.example.test","apiKey":"candidate-key","apiSecret":"candidate-secret","enabled":true}`
	request := httptest.NewRequest(http.MethodPut, "/api/komodo/connections/"+instance.ID, strings.NewReader(updateBody))
	response := httptest.NewRecorder()
	handler.HandleUpdate(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("failed rotation status = %d, body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "candidate-key") || strings.Contains(response.Body.String(), "candidate-secret") {
		t.Fatalf("failed rotation leaked candidate credentials: %s", response.Body.String())
	}
	loaded, err := persistence.LoadKomodoConfig()
	if err != nil || len(loaded) != 1 || loaded[0].APIKey != "stored-key" || loaded[0].APISecret != "stored-secret" {
		t.Fatalf("failed rotation changed stored credentials: %+v, err=%v", loaded, err)
	}
}

func TestKomodoConnectionRoutesDoNotExposeActionAuthority(t *testing.T) {
	called := false
	router := &Router{}
	handler := router.platformConnectionItemRoute("komodo_unavailable", "Komodo unavailable", func() *platformConnectionItemHandlers {
		return &platformConnectionItemHandlers{
			test:   func(http.ResponseWriter, *http.Request) { called = true },
			delete: func(http.ResponseWriter, *http.Request) { called = true },
			update: func(http.ResponseWriter, *http.Request) { called = true },
		}
	})
	request := httptest.NewRequest(http.MethodPost, "/api/komodo/connections/87ef53c3-9a5a-4d7b-a159-997ccca19811/execute", nil)
	response := httptest.NewRecorder()
	handler(response, request)
	if response.Code != http.StatusMethodNotAllowed || called {
		t.Fatalf("Komodo execute route status=%d called=%t; expected denied with no handler", response.Code, called)
	}
}
