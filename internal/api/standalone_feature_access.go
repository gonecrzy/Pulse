package api

import (
	"context"
	"fmt"

	"github.com/rcourtman/pulse-go-rewrite/internal/forkfeatures"
)

// standaloneFeatureChecker substitutes only explicitly mapped, implemented
// public capabilities with the local fork policy. All other features retain
// the normal licensing decision. It never changes commercial status payloads.
type standaloneFeatureChecker struct {
	base       licenseFeatureChecker
	access     forkfeatures.AccessResolver
	ctx        context.Context
	standalone bool
}

type localForkCapabilityUnavailableError struct {
	Capability forkfeatures.FeatureID
	Cause      error
}

func (e *localForkCapabilityUnavailableError) Error() string {
	return fmt.Sprintf("local fork capability %q is unavailable", e.Capability)
}

func (e *localForkCapabilityUnavailableError) Unwrap() error { return e.Cause }

func (c standaloneFeatureChecker) HasFeature(feature string) bool {
	id, mapped := forkfeatures.LocalCapabilityForLicenseFeature(feature)
	if c.standalone && mapped {
		return c.access.Resolve(c.ctx, id).Available
	}
	return c.base != nil && c.base.HasFeature(feature)
}

func (c standaloneFeatureChecker) RequireFeature(feature string) error {
	id, mapped := forkfeatures.LocalCapabilityForLicenseFeature(feature)
	if c.standalone && mapped {
		if _, err := c.access.Require(c.ctx, id); err != nil {
			return &localForkCapabilityUnavailableError{Capability: id, Cause: err}
		}
		return nil
	}
	if c.base == nil {
		return fmt.Errorf("license service unavailable")
	}
	return c.base.RequireFeature(feature)
}
