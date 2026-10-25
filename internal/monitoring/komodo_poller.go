package monitoring

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rcourtman/pulse-go-rewrite/internal/config"
	komodocore "github.com/rcourtman/pulse-go-rewrite/internal/komodo"
	"github.com/rcourtman/pulse-go-rewrite/internal/unifiedresources"
	"github.com/rs/zerolog/log"
)

const defaultKomodoPollInterval = 60 * time.Second

type komodoInventoryClient interface {
	CollectInventory(context.Context) (komodocore.Inventory, error)
}

type komodoClientFactory func(config.KomodoInstance) (komodoInventoryClient, error)

// KomodoPollError exposes only a safe category and fixed message. Upstream
// response bodies and connection details are intentionally discarded.
type KomodoPollError struct {
	At       *time.Time `json:"at,omitempty"`
	Category string     `json:"category"`
	Message  string     `json:"message"`
}

// KomodoConnectionPollStatus is a redacted view of one configured connection.
type KomodoConnectionPollStatus struct {
	Enabled             bool             `json:"enabled"`
	State               string           `json:"state"`
	ResourceCount       int              `json:"resourceCount"`
	LastAttemptAt       *time.Time       `json:"lastAttemptAt,omitempty"`
	LastSuccessAt       *time.Time       `json:"lastSuccessAt,omitempty"`
	ConsecutiveFailures int              `json:"consecutiveFailures,omitempty"`
	LastError           *KomodoPollError `json:"lastError,omitempty"`
}

type komodoConnectionRuntimeStatus struct {
	lastAttemptAt       time.Time
	lastSuccessAt       time.Time
	consecutiveFailures int
	lastError           *KomodoPollError
	resourceCount       int
	disabled            bool
}

// KomodoPoller independently collects Komodo inventory per organization and
// publishes it through Pulse's existing supplemental-record provider path.
// It has no dependency on AI, alerting, or action execution.
type KomodoPoller struct {
	multiTenant   *config.MultiTenantPersistence
	mu            sync.Mutex
	clientsByOrg  map[string]map[string]komodoInventoryClient
	configsByOrg  map[string]map[string]config.KomodoInstance
	statusByOrg   map[string]map[string]*komodoConnectionRuntimeStatus
	recordsByOrg  map[string]map[string][]unifiedresources.IngestRecord
	clientFactory komodoClientFactory
	interval      time.Duration
	cancel        context.CancelFunc
	stopped       chan struct{}
}

// NewKomodoPoller constructs an org-scoped poller using the native bounded
// Komodo v2 API client.
func NewKomodoPoller(multiTenant *config.MultiTenantPersistence, interval time.Duration) *KomodoPoller {
	if interval <= 0 {
		interval = defaultKomodoPollInterval
	}
	stopped := make(chan struct{})
	close(stopped)
	return &KomodoPoller{
		multiTenant:  multiTenant,
		clientsByOrg: make(map[string]map[string]komodoInventoryClient),
		configsByOrg: make(map[string]map[string]config.KomodoInstance),
		statusByOrg:  make(map[string]map[string]*komodoConnectionRuntimeStatus),
		recordsByOrg: make(map[string]map[string][]unifiedresources.IngestRecord),
		clientFactory: func(instance config.KomodoInstance) (komodoInventoryClient, error) {
			if err := instance.Validate(); err != nil {
				return nil, errors.New("invalid Komodo connection configuration")
			}
			return komodocore.NewClient(komodocore.ClientConfig{
				Endpoint:  instance.Endpoint,
				APIKey:    instance.APIKey,
				APISecret: instance.APISecret,
			})
		},
		interval: interval,
		stopped:  stopped,
	}
}

// Start starts the shared platform-poller loop. Komodo has no feature flag or
// AI gate; an empty config simply produces no active work.
func (p *KomodoPoller) Start(ctx context.Context) {
	if p == nil {
		return
	}
	startPollerLoop(ctx, &p.mu, &p.cancel, &p.stopped, p.syncConnections, p.pollAll, p.nextWaitDuration)
}

// Stop cancels the polling loop and waits up to five seconds for it to exit.
func (p *KomodoPoller) Stop() {
	if p == nil {
		return
	}
	p.mu.Lock()
	cancel, stopped := p.cancel, p.stopped
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if stopped == nil {
		return
	}
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-stopped:
	case <-timer.C:
		log.Warn().Str("component", "komodo_poller").Str("action", "stop").Msg("Komodo poller stop timed out")
	}
}

func (p *KomodoPoller) nextWaitDuration(time.Time) time.Duration {
	if p == nil || p.interval <= 0 {
		return defaultKomodoPollInterval
	}
	return p.interval
}

func (p *KomodoPoller) syncConnections() {
	if p == nil || p.multiTenant == nil {
		return
	}
	orgIDs, err := p.multiTenant.ListOrganizationIDs()
	if err != nil {
		log.Warn().Str("component", "komodo_poller").Str("action", "list_organizations").Msg("Komodo poller could not enumerate organizations")
		return
	}
	configured := make(map[string]map[string]config.KomodoInstance, len(orgIDs))
	loadFailed := make(map[string]bool)
	for _, rawOrgID := range orgIDs {
		orgID := normalizeKomodoOrgID(rawOrgID)
		persistence, err := p.multiTenant.GetPersistence(orgID)
		if err != nil || persistence == nil {
			loadFailed[orgID] = true
			continue
		}
		instances, err := persistence.LoadKomodoConfig()
		if err != nil {
			loadFailed[orgID] = true
			continue
		}
		configured[orgID] = make(map[string]config.KomodoInstance)
		for _, instance := range instances {
			instance.ID = strings.TrimSpace(instance.ID)
			if instance.ID == "" {
				continue
			}
			configured[orgID][instance.ID] = instance
		}
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	for orgID, byConn := range configured {
		if p.configsByOrg[orgID] == nil {
			p.configsByOrg[orgID] = make(map[string]config.KomodoInstance)
			p.clientsByOrg[orgID] = make(map[string]komodoInventoryClient)
			p.statusByOrg[orgID] = make(map[string]*komodoConnectionRuntimeStatus)
			p.recordsByOrg[orgID] = make(map[string][]unifiedresources.IngestRecord)
		}
		for connID, instance := range byConn {
			if p.statusByOrg[orgID][connID] == nil {
				p.statusByOrg[orgID][connID] = &komodoConnectionRuntimeStatus{}
			}
			status := p.statusByOrg[orgID][connID]
			status.disabled = !instance.Enabled
			if !instance.Enabled {
				delete(p.clientsByOrg[orgID], connID)
				delete(p.recordsByOrg[orgID], connID)
				status.resourceCount = 0
				status.lastError = nil
				continue
			}
			previous, exists := p.configsByOrg[orgID][connID]
			if exists && previous == instance && p.clientsByOrg[orgID][connID] != nil {
				continue
			}
			client, err := p.clientFactory(instance)
			if err != nil {
				delete(p.clientsByOrg[orgID], connID)
				p.recordKomodoFailureLocked(orgID, connID, err, time.Now().UTC())
				continue
			}
			p.configsByOrg[orgID][connID] = instance
			p.clientsByOrg[orgID][connID] = client
		}
	}
	for orgID := range p.clientsByOrg {
		if loadFailed[orgID] {
			continue
		}
		byConn := configured[orgID]
		for connID := range p.clientsByOrg[orgID] {
			instance, exists := byConn[connID]
			if !exists || !instance.Enabled {
				delete(p.clientsByOrg[orgID], connID)
				delete(p.configsByOrg[orgID], connID)
				delete(p.recordsByOrg[orgID], connID)
			}
		}
		for connID := range p.configsByOrg[orgID] {
			if _, exists := byConn[connID]; !exists {
				delete(p.configsByOrg[orgID], connID)
				delete(p.recordsByOrg[orgID], connID)
			}
		}
		for connID := range p.statusByOrg[orgID] {
			if _, exists := byConn[connID]; !exists {
				delete(p.statusByOrg[orgID], connID)
			}
		}
		if len(byConn) == 0 {
			delete(p.clientsByOrg, orgID)
			delete(p.configsByOrg, orgID)
			delete(p.recordsByOrg, orgID)
			delete(p.statusByOrg, orgID)
		}
	}
	for orgID := range p.statusByOrg {
		if !loadFailed[orgID] {
			if _, stillExists := configured[orgID]; !stillExists {
				delete(p.statusByOrg, orgID)
			}
		}
	}
}

func (p *KomodoPoller) pollAll(ctx context.Context) {
	if p == nil {
		return
	}
	type entry struct {
		orgID  string
		connID string
		client komodoInventoryClient
	}
	p.mu.Lock()
	entries := make([]entry, 0)
	for orgID, clients := range p.clientsByOrg {
		for connID, client := range clients {
			if client != nil {
				entries = append(entries, entry{orgID: orgID, connID: connID, client: client})
			}
		}
	}
	p.mu.Unlock()
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].orgID == entries[j].orgID {
			return entries[i].connID < entries[j].connID
		}
		return entries[i].orgID < entries[j].orgID
	})

	for _, entry := range entries {
		inventory, err := entry.client.CollectInventory(ctx)
		completed := time.Now().UTC()
		p.mu.Lock()
		if p.clientsByOrg[entry.orgID][entry.connID] != entry.client {
			p.mu.Unlock()
			continue
		}
		if err != nil {
			p.recordKomodoFailureLocked(entry.orgID, entry.connID, err, completed)
			p.mu.Unlock()
			continue
		}
		status := p.ensureKomodoStatusLocked(entry.orgID, entry.connID)
		status.lastAttemptAt = completed
		status.lastSuccessAt = completed
		status.consecutiveFailures = 0
		status.lastError = nil
		instance := p.configsByOrg[entry.orgID][entry.connID]
		records := komodocore.NormalizeInventory(instance.ID, inventory, completed)
		p.recordsByOrg[entry.orgID][entry.connID] = records
		status.resourceCount = len(records)
		p.mu.Unlock()
	}
}

func (p *KomodoPoller) recordKomodoFailureLocked(orgID, connID string, err error, at time.Time) {
	status := p.ensureKomodoStatusLocked(orgID, connID)
	status.lastAttemptAt = at
	status.consecutiveFailures++
	category, message := safeKomodoPollError(err)
	status.lastError = &KomodoPollError{At: komodoTimePointer(at), Category: category, Message: message}
}

func safeKomodoPollError(err error) (string, string) {
	if err == nil {
		return "collection", "Collection failed"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "auth"):
		return "authentication", "Authentication failed"
	case strings.Contains(message, "permission"), strings.Contains(message, "scope"):
		return "scope", "Read permission scope could not be verified"
	case strings.Contains(message, "version"):
		return "compatibility", "Komodo Core API version is unsupported"
	case strings.Contains(message, "timeout"):
		return "timeout", "Komodo API request timed out"
	case strings.Contains(message, "upstream"):
		return "upstream", "Komodo API request failed"
	default:
		return "collection", "Komodo inventory collection failed"
	}
}

func normalizeKomodoOrgID(orgID string) string {
	if strings.TrimSpace(orgID) == "" {
		return "default"
	}
	return strings.TrimSpace(orgID)
}

func (p *KomodoPoller) ensureKomodoStatusLocked(orgID, connID string) *komodoConnectionRuntimeStatus {
	if p.statusByOrg[orgID] == nil {
		p.statusByOrg[orgID] = make(map[string]*komodoConnectionRuntimeStatus)
	}
	if p.statusByOrg[orgID][connID] == nil {
		p.statusByOrg[orgID][connID] = &komodoConnectionRuntimeStatus{}
	}
	return p.statusByOrg[orgID][connID]
}

func komodoTimePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	copy := value
	return &copy
}

func (p *KomodoPoller) GetCurrentRecords() []unifiedresources.IngestRecord {
	return p.GetCurrentRecordsForOrg("default")
}

// GetCurrentRecordsForOrg returns cloned last-known records for one org.
func (p *KomodoPoller) GetCurrentRecordsForOrg(orgID string) []unifiedresources.IngestRecord {
	if p == nil {
		return nil
	}
	orgID = normalizeKomodoOrgID(orgID)
	p.mu.Lock()
	defer p.mu.Unlock()
	byConn := p.recordsByOrg[orgID]
	connIDs := make([]string, 0, len(byConn))
	for connID := range byConn {
		connIDs = append(connIDs, connID)
	}
	sort.Strings(connIDs)
	var records []unifiedresources.IngestRecord
	for _, connID := range connIDs {
		for _, record := range byConn[connID] {
			record.Resource.Relationships = append([]unifiedresources.ResourceRelationship(nil), record.Resource.Relationships...)
			record.Resource.Sources = append([]unifiedresources.DataSource(nil), record.Resource.Sources...)
			records = append(records, record)
		}
	}
	return records
}

// SupplementalRecords adapts Komodo cache records to the existing monitor
// publication contract.
func (p *KomodoPoller) SupplementalRecords(_ *Monitor, orgID string) []unifiedresources.IngestRecord {
	return p.GetCurrentRecordsForOrg(orgID)
}

// SnapshotOwnedSources declares Komodo's source-native inventory ownership.
func (*KomodoPoller) SnapshotOwnedSources() []unifiedresources.DataSource {
	return []unifiedresources.DataSource{unifiedresources.SourceKomodo}
}

// SnapshotOwnedSourcesForOrg is the org-aware ownership variant.
func (p *KomodoPoller) SnapshotOwnedSourcesForOrg(string) []unifiedresources.DataSource {
	return p.SnapshotOwnedSources()
}

// ConnectionSummaries returns poll state without endpoint or credentials.
func (p *KomodoPoller) ConnectionSummaries(orgID string, instances []config.KomodoInstance) map[string]KomodoConnectionPollStatus {
	if len(instances) == 0 {
		return nil
	}
	orgID = normalizeKomodoOrgID(orgID)
	summaries := make(map[string]KomodoConnectionPollStatus, len(instances))
	if p == nil {
		for _, instance := range instances {
			summaries[instance.ID] = KomodoConnectionPollStatus{Enabled: instance.Enabled, State: "pending"}
		}
		return summaries
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, instance := range instances {
		id := strings.TrimSpace(instance.ID)
		if id == "" {
			continue
		}
		status := p.statusByOrg[orgID][id]
		if !instance.Enabled {
			summaries[id] = KomodoConnectionPollStatus{Enabled: false, State: "disabled"}
			continue
		}
		copy := KomodoConnectionPollStatus{Enabled: true, State: "pending"}
		if status != nil {
			copy.ResourceCount = status.resourceCount
			copy.LastAttemptAt = komodoTimePointer(status.lastAttemptAt)
			copy.LastSuccessAt = komodoTimePointer(status.lastSuccessAt)
			copy.ConsecutiveFailures = status.consecutiveFailures
			if status.lastError != nil {
				errCopy := *status.lastError
				if errCopy.At != nil {
					errCopy.At = komodoTimePointer(*errCopy.At)
				}
				copy.LastError = &errCopy
			}
			switch {
			case status.consecutiveFailures > 0 && !status.lastSuccessAt.IsZero():
				copy.State = "stale"
			case status.consecutiveFailures > 0:
				copy.State = "failed"
			case !status.lastSuccessAt.IsZero():
				copy.State = "healthy"
			}
		}
		summaries[id] = copy
	}
	return summaries
}
