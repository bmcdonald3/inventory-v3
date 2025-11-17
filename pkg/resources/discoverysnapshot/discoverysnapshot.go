// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package discoverysnapshot

import (
	"context"
	"github.com/openchami/fabrica/pkg/resource"
	"encoding/json"
)

// DiscoverySnapshot represents a DiscoverySnapshot resource
type DiscoverySnapshot struct {
	resource.Resource
	Spec   DiscoverySnapshotSpec   `json:"spec" validate:"required"`
	Status DiscoverySnapshotStatus `json:"status,omitempty"`
}

// DiscoverySnapshotSpec defines the desired state of DiscoverySnapshot
type DiscoverySnapshotSpec struct {
	// RawData holds the complete, raw JSON payload from a discovery tool (e.g., the collector).
	// The reconciler will parse this.
	RawData json.RawMessage `json:"rawData" validate:"required"`
}

// DiscoverySnapshotStatus defines the observed state of DiscoverySnapshot
type DiscoverySnapshotStatus struct {
	Phase      string `json:"phase,omitempty"`
	Message    string `json:"message,omitempty"`
	Ready      bool   `json:"ready"`
}

// Validate implements custom validation logic for DiscoverySnapshot
func (r *DiscoverySnapshot) Validate(ctx context.Context) error {
	// Add custom validation logic here
	// Example:
	// if r.Spec.Name == "forbidden" {
	//     return errors.New("name 'forbidden' is not allowed")
	// }

	return nil
}
// GetKind returns the kind of the resource
func (r *DiscoverySnapshot) GetKind() string {
	return "DiscoverySnapshot"
}

// GetName returns the name of the resource
func (r *DiscoverySnapshot) GetName() string {
	return r.Metadata.Name
}

// GetUID returns the UID of the resource
func (r *DiscoverySnapshot) GetUID() string {
	return r.Metadata.UID
}

func init() {
	// Register resource type prefix for storage
	resource.RegisterResourcePrefix("DiscoverySnapshot", "dis")
}
