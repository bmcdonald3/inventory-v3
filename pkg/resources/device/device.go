// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

package device

import (
	"context"
	"github.com/openchami/fabrica/pkg/resource"
)

// Device represents a Device resource
type Device struct {
	resource.Resource
	Spec   DeviceSpec   `json:"spec" validate:"required"`
	Status DeviceStatus `json:"status,omitempty"`
}

// DeviceSpec defines the desired state of Device
type DeviceSpec struct {
	Description string `json:"description,omitempty" validate:"max=200"`
	// Add your spec fields here
}

// DeviceStatus defines the observed state of Device
type DeviceStatus struct {
	Phase      string `json:"phase,omitempty"`
	Message    string `json:"message,omitempty"`
	Ready      bool   `json:"ready"`
	// Add your status fields here
}

// Validate implements custom validation logic for Device
func (r *Device) Validate(ctx context.Context) error {
	// Add custom validation logic here
	// Example:
	// if r.Spec.Name == "forbidden" {
	//     return errors.New("name 'forbidden' is not allowed")
	// }

	return nil
}
// GetKind returns the kind of the resource
func (r *Device) GetKind() string {
	return "Device"
}

// GetName returns the name of the resource
func (r *Device) GetName() string {
	return r.Metadata.Name
}

// GetUID returns the UID of the resource
func (r *Device) GetUID() string {
	return r.Metadata.UID
}

func init() {
	// Register resource type prefix for storage
	resource.RegisterResourcePrefix("Device", "dev")
}
