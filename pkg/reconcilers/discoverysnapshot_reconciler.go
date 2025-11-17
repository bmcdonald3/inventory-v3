// This file is safe to edit.
// It contains the implementation for the DiscoverySnapshot reconciler.
// Copyright © 2025 OpenCHAMI a Series of LF Projects, LLC
//
// SPDX-License-Identifier: MIT

// This file is safe to edit.
// It contains the implementation for the DiscoverySnapshot reconciler.
package reconcilers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/example/inventory-v3/pkg/resources/device"
	"github.com/example/inventory-v3/pkg/resources/discoverysnapshot"

	// Import Fabrica packages
	fabResource "github.com/openchami/fabrica/pkg/resource"
)

// reconcileDiscoverySnapshot is the core reconciliation logic for DiscoverySnapshot.
// This function is called by the generated boilerplate.
// --- THIS SIGNATURE IS NOW CORRECT: returns 'error' only ---
func (r *DiscoverySnapshotReconciler) reconcileDiscoverySnapshot(ctx context.Context, snapshot *discoverysnapshot.DiscoverySnapshot) error {
	// 1. Check if already processed
	if snapshot.Status.Phase == "Completed" {
		r.Logger.Infof("Reconciling %s: Already completed, skipping.", snapshot.GetName())
		// The wrapper will handle the requeue. We just return nil.
		return nil
	}

	r.Logger.Infof("Reconciling %s: Starting reconciliation", snapshot.GetName())

	// 2. Set phase to "Processing"
	// We just update the struct in memory. The wrapper will save it.
	snapshot.Status.Phase = "Processing"
	snapshot.Status.Message = "Reconciler has started processing the snapshot."
	snapshot.Status.Ready = false
	// --- REMOVED: r.Client.UpdateStatus(...) ---

	// 3. --- START PAYLOAD PROCESSING (TWO-PASS LOGIC) ---

	// 3a. Unmarshal the payload
	var payloadSpecs []device.DeviceSpec
	if err := json.Unmarshal(snapshot.Spec.RawData, &payloadSpecs); err != nil {
		snapshot.Status.Phase = "Error"
		snapshot.Status.Message = fmt.Sprintf("Failed to parse rawData: %v", err)
		// Return nil; this is a permanent failure, don't retry.
		return nil
	}

	// 3b. Load all existing devices from storage
	deviceMapBySerial, err := r.buildDeviceMapBySerial(ctx)
	if err != nil {
		// Return an error to retry
		return fmt.Errorf("failed to build device map: %w", err)
	}
	r.Logger.Infof("Reconciling %s: Loaded %d existing devices into map", snapshot.GetName(), len(deviceMapBySerial))

	snapshotDeviceMap := make(map[string]*device.Device)

	// --- PASS 1: CREATE AND UPDATE DEVICES ---
	processedCount := 0
	for _, spec := range payloadSpecs {
		if spec.SerialNumber == "" {
			r.Logger.Errorf("Reconciling %s: Skipping device with no serial number", snapshot.GetName())
			continue
		}

		existingDevice, found := deviceMapBySerial[spec.SerialNumber]
		if !found {
			// --- CREATE NEW DEVICE ---
			r.Logger.Infof("Reconciling %s (Pass 1): Creating new device: %s", snapshot.GetName(), spec.SerialNumber)
			newDevice, err := r.createNewDevice(ctx, spec)
			if err != nil {
				r.Logger.Errorf("Reconciling %s (Pass 1): Failed to create device %s: %v", snapshot.GetName(), spec.SerialNumber, err)
				continue
			}
			snapshotDeviceMap[newDevice.Spec.SerialNumber] = newDevice
			deviceMapBySerial[newDevice.Spec.SerialNumber] = newDevice // Add to global map

		} else {
			// --- UPDATE EXISTING DEVICE ---
			r.Logger.Infof("Reconciling %s (Pass 1): Updating existing device: %s (UID: %s)", snapshot.GetName(), spec.SerialNumber, existingDevice.GetUID())

			spec.ParentID = existingDevice.Spec.ParentID
			existingDevice.Spec = spec
			existingDevice.Metadata.UpdatedAt = time.Now()

			if err := r.Client.Update(ctx, existingDevice); err != nil {
				r.Logger.Errorf("Reconciling %s (Pass 1): Failed to update device %s: %v", snapshot.GetName(), spec.SerialNumber, err)
				continue
			}
			snapshotDeviceMap[existingDevice.Spec.SerialNumber] = existingDevice
		}
		processedCount++
	}

	// --- PASS 2: LINK PARENT IDs ---
	r.Logger.Infof("Reconciling %s (Pass 2): Linking parent relationships...", snapshot.GetName())
	linksUpdated := 0
	for _, dev := range snapshotDeviceMap {
		parentSerial := dev.Spec.ParentSerialNumber
		if parentSerial == "" {
			continue
		}
		parentDevice, found := deviceMapBySerial[parentSerial]
		if !found {
			r.Logger.Errorf("Reconciling %s (Pass 2): Parent device with serial %s not found for child %s", snapshot.GetName(), parentSerial, dev.Spec.SerialNumber)
			continue
		}
		if dev.Spec.ParentID == parentDevice.GetUID() {
			continue
		}
		r.Logger.Infof("Reconciling %s (Pass 2): Linking %s (UID: %s) to parent %s (UID: %s)",
			snapshot.GetName(), dev.Spec.SerialNumber, dev.GetUID(), parentDevice.Spec.SerialNumber, parentDevice.GetUID())

		dev.Spec.ParentID = parentDevice.GetUID()
		dev.Metadata.UpdatedAt = time.Now()

		if err := r.Client.Update(ctx, dev); err != nil {
			r.Logger.Errorf("Reconciling %s (Pass 2): Failed to update parent link for %s: %v", snapshot.GetName(), dev.Spec.SerialNumber, err)
		} else {
			linksUpdated++
		}
	}

	// 4. Set phase to "Completed"
	snapshot.Status.Phase = "Completed"
	snapshot.Status.Message = fmt.Sprintf("Snapshot processed. %d devices created/updated. %d parent links updated.", processedCount, linksUpdated)
	snapshot.Status.Ready = true
	// The generated wrapper will save this status for us.

	r.Logger.Infof("Reconciling %s: Successfully reconciled", snapshot.GetName())

	// Return nil for success
	return nil
}

// createNewDevice (This helper function is unchanged)
func (r *DiscoverySnapshotReconciler) createNewDevice(ctx context.Context, spec device.DeviceSpec) (*device.Device, error) {
	newDevice := &device.Device{
		Resource: fabResource.Resource{
			APIVersion:    "v1",
			Kind:          "Device",
			SchemaVersion: "v1",
		},
		Spec: spec,
	}

	uid, err := fabResource.GenerateUIDForResource("Device")
	if err != nil {
		return nil, fmt.Errorf("failed to generate UID for device: %w", err)
	}
	now := time.Now()
	newDevice.Metadata.UID = uid
	newDevice.Metadata.Name = spec.SerialNumber
	newDevice.Metadata.CreatedAt = now
	newDevice.Metadata.UpdatedAt = now

	if err := r.Client.Create(ctx, newDevice); err != nil {
		return nil, fmt.Errorf("failed to create device %s: %w", spec.SerialNumber, err)
	}
	return newDevice, nil
}

// buildDeviceMapBySerial (This helper function is unchanged)
func (r *DiscoverySnapshotReconciler) buildDeviceMapBySerial(ctx context.Context) (map[string]*device.Device, error) {
	resourceList, err := r.Client.List(ctx, "Device")
	if err != nil {
		return nil, err
	}
	deviceMap := make(map[string]*device.Device)
	for _, item := range resourceList {
		dev, ok := item.(*device.Device)
		if !ok {
			r.Logger.Errorf("Reconciling: Found non-device item in storage, skipping.")
			continue
		}
		if dev.Spec.SerialNumber != "" {
			deviceMap[dev.Spec.SerialNumber] = dev
		}
	}
	return deviceMap, nil
}