package monitoring

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rcourtman/pulse-go-rewrite/internal/config"
	komodocore "github.com/rcourtman/pulse-go-rewrite/internal/komodo"
	"github.com/rcourtman/pulse-go-rewrite/internal/unifiedresources"
)

type sequencedKomodoClient struct {
	result komodocore.Inventory
	err    error
}

func (c *sequencedKomodoClient) CollectInventory(context.Context) (komodocore.Inventory, error) {
	return c.result, c.err
}

func TestKomodoPollerScopesOrganizationsAndDistinguishesEmptyFailureStaleDisableAndRemoval(t *testing.T) {
	mtp, defaultPersistence := newTestTenantPersistence(t)
	orgAPersistence, err := mtp.GetPersistence("org-a")
	if err != nil {
		t.Fatalf("GetPersistence(org-a): %v", err)
	}
	orgBPersistence, err := mtp.GetPersistence("org-b")
	if err != nil {
		t.Fatalf("GetPersistence(org-b): %v", err)
	}
	orgA := config.NewKomodoInstance()
	orgA.Name, orgA.Endpoint, orgA.APIKey, orgA.APISecret = "A", "https://a.example.test", "key-a", "secret-a"
	orgB := config.NewKomodoInstance()
	orgB.Name, orgB.Endpoint, orgB.APIKey, orgB.APISecret = "B", "https://b.example.test", "key-b", "secret-b"
	if err := orgAPersistence.SaveKomodoConfig([]config.KomodoInstance{orgA}); err != nil {
		t.Fatalf("SaveKomodoConfig(org-a): %v", err)
	}
	if err := orgBPersistence.SaveKomodoConfig([]config.KomodoInstance{orgB}); err != nil {
		t.Fatalf("SaveKomodoConfig(org-b): %v", err)
	}
	if err := defaultPersistence.SaveKomodoConfig(nil); err != nil {
		t.Fatalf("SaveKomodoConfig(default): %v", err)
	}

	clientA := &sequencedKomodoClient{result: komodocore.Inventory{Servers: []komodocore.Resource{{ID: "server-1", Name: "org-a-host", Kind: komodocore.KindServer, State: "Ok"}}}}
	clientB := &sequencedKomodoClient{result: komodocore.Inventory{Servers: []komodocore.Resource{{ID: "server-1", Name: "org-b-host", Kind: komodocore.KindServer, State: "Ok"}}}}
	poller := NewKomodoPoller(mtp, time.Hour)
	poller.clientFactory = func(instance config.KomodoInstance) (komodoInventoryClient, error) {
		switch instance.Endpoint {
		case orgA.Endpoint:
			return clientA, nil
		case orgB.Endpoint:
			return clientB, nil
		default:
			return nil, errors.New("unexpected test endpoint")
		}
	}
	poller.syncConnections()
	poller.pollAll(context.Background())

	orgARecords := poller.GetCurrentRecordsForOrg("org-a")
	orgBRecords := poller.GetCurrentRecordsForOrg("org-b")
	if len(orgARecords) != 1 || len(orgBRecords) != 1 {
		t.Fatalf("org record counts = %d and %d, want one each", len(orgARecords), len(orgBRecords))
	}
	if orgARecords[0].Resource.Name != "org-a-host" || orgBRecords[0].Resource.Name != "org-b-host" {
		t.Fatalf("org records crossed boundaries: A=%q B=%q", orgARecords[0].Resource.Name, orgBRecords[0].Resource.Name)
	}
	if orgARecords[0].SourceID == orgBRecords[0].SourceID {
		t.Fatal("same Komodo native ID was not scoped to its connection")
	}
	if got := poller.ConnectionSummaries("org-a", []config.KomodoInstance{orgA})[orgA.ID].State; got != "healthy" {
		t.Fatalf("initial state = %q, want healthy", got)
	}

	clientA.err = errors.New("upstream response contained secret-a and private config")
	poller.pollAll(context.Background())
	stale := poller.ConnectionSummaries("org-a", []config.KomodoInstance{orgA})[orgA.ID]
	if stale.State != "stale" || stale.ResourceCount != 1 || len(poller.GetCurrentRecordsForOrg("org-a")) != 1 {
		t.Fatalf("failed collection did not retain stale last-known inventory: %#v", stale)
	}
	if stale.LastError == nil || strings.Contains(stale.LastError.Message, "secret-a") || strings.Contains(stale.LastError.Message, "private config") {
		t.Fatalf("poll status exposed upstream response material: %#v", stale.LastError)
	}

	clientA.err = nil
	clientA.result = komodocore.Inventory{}
	poller.pollAll(context.Background())
	empty := poller.ConnectionSummaries("org-a", []config.KomodoInstance{orgA})[orgA.ID]
	if empty.State != "healthy" || empty.ResourceCount != 0 || len(poller.GetCurrentRecordsForOrg("org-a")) != 0 {
		t.Fatalf("successful empty inventory was not published distinctly: %#v", empty)
	}
	if len(poller.GetCurrentRecordsForOrg("org-b")) != 1 {
		t.Fatal("org-a success replaced org-b's inventory")
	}

	clientA.result = komodocore.Inventory{Servers: []komodocore.Resource{{ID: "server-1", Name: "org-a-host", Kind: komodocore.KindServer, State: "Ok"}}}
	poller.pollAll(context.Background())
	orgA.Enabled = false
	if err := orgAPersistence.SaveKomodoConfig([]config.KomodoInstance{orgA}); err != nil {
		t.Fatalf("disable Komodo connection: %v", err)
	}
	poller.syncConnections()
	disabled := poller.ConnectionSummaries("org-a", []config.KomodoInstance{orgA})[orgA.ID]
	if disabled.State != "disabled" || disabled.Enabled || len(poller.GetCurrentRecordsForOrg("org-a")) != 0 {
		t.Fatalf("intentional disablement retained active inventory: %#v", disabled)
	}

	if err := orgAPersistence.SaveKomodoConfig(nil); err != nil {
		t.Fatalf("remove Komodo connection: %v", err)
	}
	poller.syncConnections()
	poller.mu.Lock()
	_, statusExists := poller.statusByOrg["org-a"][orgA.ID]
	poller.mu.Unlock()
	if statusExists {
		t.Fatal("removed connection retained runtime status")
	}
	if len(poller.GetCurrentRecordsForOrg("org-b")) != 1 {
		t.Fatal("removing org-a connection changed org-b inventory")
	}

	if got := poller.SnapshotOwnedSourcesForOrg("org-b"); len(got) != 1 || got[0] != unifiedresources.SourceKomodo {
		t.Fatalf("owned sources = %v, want only Komodo", got)
	}
}
