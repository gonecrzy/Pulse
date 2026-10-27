package migration

import (
	"encoding/json"
	"net/http"

	"github.com/rcourtman/pulse-go-rewrite/internal/api"
	"github.com/rcourtman/pulse-go-rewrite/internal/config"
	pkglicensing "github.com/rcourtman/pulse-go-rewrite/pkg/licensing"
)

func newCommercialMigrationLicenseHandlers(mtp *config.MultiTenantPersistence) *api.LicenseHandlers {
	return api.NewLicenseHandlers(mtp, false, &config.Config{UpstreamCommercialServicesEnabled: true})
}

func handleMigrationInstallationStatus(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/v1/grants/status" {
		return false
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return true
	}

	var req pkglicensing.InstallationStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return true
	}
	licenseVersion := req.CurrentLicenseVersion
	if licenseVersion <= 0 {
		licenseVersion = 1
	}
	_ = json.NewEncoder(w).Encode(pkglicensing.InstallationStatusResponse{
		LicenseVersion: licenseVersion,
		StatusPolicy: pkglicensing.StatusHints{
			IntervalSeconds: 300,
			JitterPercent:   20,
		},
	})
	return true
}
