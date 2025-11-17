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

	"github.com/example/inventory-api/pkg/resources/device"
	"github.com/example/inventory-api/pkg/resources/discoverysnapshot"
	"github.com/openchami/fabrica/pkg/reconcile"
	fabResource "github.com/openchami/fabrica/pkg/resource"
)

// reconcileDiscoverySnapshot is the core reconciliation logic for DiscoverySnapshot.
func (r *DiscoverySnapshotReconciler) reconcileDiscoverySnapshot(ctx context.Context, snapshot *discoverysnapshot.DiscoverySnapshot) error {
	if snapshot.Status.Phase == "Completed" {
		r.Logger.Infof("Reconciling %s: Already completed, skipping.", snapshot.GetName())
		return nil
	}

	r.Logger.Infof("Reconciling %s: Starting reconciliation", snapshot.GetName())
	snapshot.Status.Phase = "Processing"
	snapshot.Status.Message = "Reconciler has started processing the snapshot."
	snapshot.Status.Ready = false

	var payloadSpecs []device.DeviceSpec
	if err := json.Unmarshal(snapshot.Spec.RawData, &payloadSpecs); err != nil {
		snapshot.Status.Phase = "Error"
		snapshot.Status.Message = fmt.Sprintf("Failed to parse rawData: %v", err)
		return nil
	}

	// --- CHANGE: We now build TWO maps ---
	// 1. A map by Redfish URI, used as the primary key for get-or-create
	deviceMapByURI, err := r.buildDeviceMapByURI(ctx)
	if err != nil {
		return fmt.Errorf("failed to build device map by URI: %w", err)
	}
	// 2. A map by Serial Number, used ONLY for parent linking in Pass 2
	deviceMapBySerial, err := r.buildDeviceMapBySerial(ctx)
	if err != nil {
		return fmt.Errorf("failed to build device map by Serial: %w", err)
	}
	// --- END CHANGE ---

	r.Logger.Infof("Reconciling %s: Loaded %d devices by URI and %d by Serial", snapshot.GetName(), len(deviceMapByURI), len(deviceMapBySerial))
	snapshotDeviceMap := make(map[string]*device.Device)
	processedCount := 0

	// --- PASS 1: CREATE AND UPDATE DEVICES (USING REDFISH URI) ---
	for _, spec := range payloadSpecs {
		// --- CHANGE: Use redfish_uri as the primary key ---
		uri, err := getRedfishURI(spec)
		if err != nil {
			r.Logger.Errorf("Reconciling %s: Skipping device, missing redfish_uri", snapshot.GetName())
			continue
		}
		// --- END CHANGE ---

		existingDevice, found := deviceMapByURI[uri]
		if !found {
			// --- CREATE NEW DEVICE ---
			r.Logger.Infof("Reconciling %s (Pass 1): Creating new device: %s", snapshot.GetName(), uri)
			// --- CHANGE: Pass URI to be used as the 'Name' ---
			newDevice, err := r.createNewDevice(ctx, spec, uri)
			if err != nil {
				r.Logger.Errorf("Reconciling %s (Pass 1): Failed to create device %s: %v", snapshot.GetName(), uri, err)
				continue
			}
			snapshotDeviceMap[uri] = newDevice
			deviceMapByURI[uri] = newDevice // Add to maps
			if newDevice.Spec.SerialNumber != "" {
				deviceMapBySerial[newDevice.Spec.SerialNumber] = newDevice
			}

		} else {
			// --- UPDATE EXISTING DEVICE ---
			r.Logger.Infof("Reconciling %s (Pass 1): Updating existing device: %s (UID: %s)", snapshot.GetName(), uri, existingDevice.GetUID())

			spec.ParentID = existingDevice.Spec.ParentID
			existingDevice.Spec = spec
			existingDevice.Metadata.UpdatedAt = time.Now()

			if err := r.Client.Update(ctx, existingDevice); err != nil {
				r.Logger.Errorf("Reconciling %s (Pass 1): Failed to update device %s: %v", snapshot.GetName(), uri, err)
				continue
			}
			snapshotDeviceMap[uri] = existingDevice
		}
		processedCount++
	}

	// --- PASS 2: LINK PARENT IDs (USING SERIAL NUMBER) ---
	// This logic is unchanged, as it relies on the serial number map
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
			snapshot.GetName(), dev.GetName(), dev.GetUID(), parentDevice.GetName(), parentDevice.GetUID())

		dev.Spec.ParentID = parentDevice.GetUID()
		dev.Metadata.UpdatedAt = time.Now()

		if err := r.Client.Update(ctx, dev); err != nil {
			r.Logger.Errorf("Reconciling %s (Pass 2): Failed to update parent link for %s: %v", snapshot.GetName(), dev.GetName(), err)
		} else {
			linksUpdated++
		}
	}

	// 4. Set phase to "Completed"
	snapshot.Status.Phase = "Completed"
	snapshot.Status.Message = fmt.Sprintf("Snapshot processed. %d devices created/updated. %d parent links updated.", processedCount, linksUpdated)
	snapshot.Status.Ready = true

	r.Logger.Infof("Reconciling %s: Successfully reconciled", snapshot.GetName())
	return nil
}

// --- THIS HELPER IS UPDATED ---
// It now takes the redfishURI to use as the Metadata.Name
func (r *DiscoverySnapshotReconciler) createNewDevice(ctx context.Context, spec device.DeviceSpec, redfishURI string) (*device.Device, error) {
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
	newDevice.Metadata.Name = redfishURI // <-- Use the unique URI as the name
	newDevice.Metadata.CreatedAt = now
	newDevice.Metadata.UpdatedAt = now

	if err := r.Client.Create(ctx, newDevice); err != nil {
		return nil, fmt.Errorf("failed to create device %s: %w", redfishURI, err)
	}
	return newDevice, nil
}

// --- THIS HELPER IS UNCHANGED ---
// We still need it for Pass 2
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

// --- NEW HELPER FUNCTION ---
// buildDeviceMapByURI fetches all devices and creates a map of [RedfishURI] -> *Device
func (r *DiscoverySnapshotReconciler) buildDeviceMapByURI(ctx context.Context) (map[string]*device.Device, error) {
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
		uri, err := getRedfishURI(dev.Spec)
		if err != nil {
			r.Logger.Warnf("Reconciling: Device %s has no redfish_uri, skipping from URI map.", dev.GetUID())
			continue
		}
		deviceMap[uri] = dev
	}
	return deviceMap, nil
}

// --- NEW HELPER FUNCTION ---
// getRedfishURI extracts the redfish_uri string from the properties map
func getRedfishURI(spec device.DeviceSpec) (string, error) {
	uriBytes, ok := spec.Properties["redfish_uri"]
	if !ok {
		return "", fmt.Errorf("missing redfish_uri in properties")
	}
	
	var uri string
	// The property is stored as a JSON string (e.g., "\"/Systems/...""),
	// so we must unmarshal it to get the raw string.
	if err := json.Unmarshal(uriBytes, &uri); err != nil {
		return "", fmt.Errorf("failed to unmarshal redfish_uri: %w", err)
	}

	if uri == "" {
		return "", fmt.Errorf("redfish_uri property is an empty string")
	}

	return uri, nil
}