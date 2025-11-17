// This file contains the complete Redfish discovery and API posting logic.
package collector

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"time"

	fabricaclient "github.com/example/inventory-v3/pkg/client"
	"github.com/example/inventory-v3/pkg/resources/device"
	"github.com/example/inventory-v3/pkg/resources/discoverysnapshot"
)

// --- Configuration ---

// InventoryAPIHost is the address of the Fabrica API server.
const InventoryAPIHost = "http://localhost:8081" // Your server runs on 8081

// DefaultUsername and DefaultPassword are hardcoded for Redfish basic auth.
const DefaultUsername = "root"
const DefaultPassword = "initial0" // Make sure this is your correct password

// --- Main Orchestration Function ---

// CollectAndPost is the main function for the collector.
func CollectAndPost(bmcIP string) error {
	// 1. Initialize Redfish Client
	rfClient, err := NewRedfishClient(bmcIP, DefaultUsername, DefaultPassword)
	if err != nil {
		return fmt.Errorf("failed to initialize Redfish client: %w", err)
	}

	fmt.Println("Starting Redfish discovery...")

	// --- 2. REDFISH DISCOVERY (Live Call) ---
	deviceSpecs, err := discoverDevices(rfClient)
	if err != nil {
		return fmt.Errorf("redfish discovery failed: %w", err)
	}
	if len(deviceSpecs) == 0 {
		return errors.New("redfish discovery found no devices to post")
	}
	fmt.Printf("Redfish Discovery Complete: Found %d total devices.\n", len(deviceSpecs))

	// --- 3. PREPARE SNAPSHOT PAYLOAD ---
	snapshotData, err := json.Marshal(deviceSpecs)
	if err != nil {
		return fmt.Errorf("failed to marshal device list into snapshot data: %w", err)
	}

	// --- 4. INITIALIZE API CLIENT (THE SDK) ---
	sdkClient, err := fabricaclient.NewClient(InventoryAPIHost, nil)
	if err != nil {
		return fmt.Errorf("failed to create fabrica client: %w", err)
	}
	ctx := context.Background()

	// --- 5. POST THE SNAPSHOT ---
	fmt.Println("Creating new DiscoverySnapshot resource...")

	// Create the Spec for the new snapshot
	snapshotSpec := discoverysnapshot.DiscoverySnapshotSpec{
		RawData: json.RawMessage(snapshotData),
	}

	// The generated CreateDiscoverySnapshotRequest struct embeds the Spec struct
	createReq := fabricaclient.CreateDiscoverySnapshotRequest{
		Name:                  fmt.Sprintf("snapshot-%s-%d", bmcIP, time.Now().Unix()),
		DiscoverySnapshotSpec: snapshotSpec, // Use the embedded struct
	}

	// Use the SDK to create the snapshot resource
	createdSnapshot, err := sdkClient.CreateDiscoverySnapshot(ctx, createReq)
	if err != nil {
		return fmt.Errorf("failed to create snapshot: %w", err)
	}

	fmt.Printf("Successfully created snapshot with UID: %s\n", createdSnapshot.Metadata.UID)
	fmt.Println("The server reconciler will now process this snapshot.")

	return nil
}

// --- Redfish Client Struct and Methods ---

// NewRedfishClient initializes the client with a specified BMC IP.
func NewRedfishClient(bmcIP, username, password string) (*RedfishClient, error) {
	baseURL := fmt.Sprintf("https://%s/redfish/v1", bmcIP)
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	return &RedfishClient{
		BaseURL:    baseURL,
		Username:   username,
		Password:   password,
		HTTPClient: &http.Client{Transport: tr},
	}, nil
}

// Get makes an authenticated GET request to a Redfish path.
func (c *RedfishClient) Get(path string) ([]byte, error) {
	targetURL, err := url.JoinPath(c.BaseURL, path)
	if err != nil {
		return nil, fmt.Errorf("failed to join path: %w", err)
	}
	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create Redfish request for %s: %w", targetURL, err)
	}
	req.SetBasicAuth(c.Username, c.Password)
	req.Header.Add("Accept", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute Redfish request for %s: %w", targetURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Redfish API returned status code %d for %s", resp.StatusCode, targetURL)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	return body, nil
}

// --- Redfish Discovery and Mapping Functions ---

// discoverDevices uses the Redfish client to walk the resource hierarchy.
func discoverDevices(c *RedfishClient) ([]*device.DeviceSpec, error) {
	var specs []*device.DeviceSpec

	systemsBody, err := c.Get("/Systems")
	if err != nil {
		return nil, fmt.Errorf("failed to get Systems collection: %w", err)
	}
	var systemsCollection RedfishCollection
	if err := json.Unmarshal(systemsBody, &systemsCollection); err != nil {
		return nil, fmt.Errorf("failed to decode Systems collection: %w", err)
	}

	for _, member := range systemsCollection.Members {
		systemURI := strings.TrimPrefix(member.ODataID, "/redfish/v1")

		systemBody, err := c.Get(systemURI)
		if err != nil {
			fmt.Printf("Warning: Failed to get system %s: %v\n", member.ODataID, err)
			continue
		}
		var systemData RedfishSystem
		if err := json.Unmarshal(systemBody, &systemData); err != nil {
			fmt.Printf("Warning: Failed to decode system data from %s: %v\n", systemURI, err)
			continue
		}

		systemInventory, err := getSystemInventory(c, systemURI, &systemData)
		if err != nil {
			fmt.Printf("Warning: Failed to get inventory for system %s: %v\n", member.ODataID, err)
			continue
		}

		// Add the Node's spec
		specs = append(specs, systemInventory.NodeSpec)
		// Add all child specs
		specs = append(specs, systemInventory.CPUs...)
		specs = append(specs, systemInventory.DIMMs...)
	}
	return specs, nil
}

// getSystemInventory discovers a single system (Node) and its children.
func getSystemInventory(c *RedfishClient, systemURI string, systemData *RedfishSystem) (*SystemInventory, error) {
	inv := &SystemInventory{CPUs: make([]*device.DeviceSpec, 0), DIMMs: make([]*device.DeviceSpec, 0)}

	// Map Node Data
	inv.NodeSpec = mapCommonProperties(
		systemData.CommonRedfishProperties,
		"Node",
		systemURI,
		"", // Node has no parent URI
		"", // Node has no parent Serial
	)

	// Get Processors (CPUs)
	if cpuCollectionURI := systemData.Processors.ODataID; cpuCollectionURI != "" {
		cleanedURI := strings.TrimPrefix(cpuCollectionURI, "/redfish/v1")
		// Pass the Node's Serial Number as the parent identifier
		cpuDevices, err := getCollectionDevices(c, cleanedURI, "CPU", systemURI, systemData.SerialNumber, &RedfishProcessor{})
		if err != nil {
			fmt.Printf("Warning: Failed to retrieve CPU inventory from %s: %v\n", cpuCollectionURI, err)
		} else {
			inv.CPUs = cpuDevices
		}
	}
	// Get Memory (DIMMs)
	if dimmCollectionURI := systemData.Memory.ODataID; dimmCollectionURI != "" {
		cleanedURI := strings.TrimPrefix(dimmCollectionURI, "/redfish/v1")
		// Pass the Node's Serial Number as the parent identifier
		dimmDevices, err := getCollectionDevices(c, cleanedURI, "DIMM", systemURI, systemData.SerialNumber, &RedfishMemory{})
		if err != nil {
			fmt.Printf("Warning: Failed to retrieve DIMM inventory from %s: %v\n", dimmCollectionURI, err)
		} else {
			inv.DIMMs = dimmDevices
		}
	}
	return inv, nil
}

// getCollectionDevices retrieves a collection, iterates over members, and maps them.
func getCollectionDevices(c *RedfishClient, collectionURI, deviceType, parentURI, parentSerial string, componentTypeExample interface{}) ([]*device.DeviceSpec, error) {
	var specs []*device.DeviceSpec
	collectionBody, err := c.Get(collectionURI)
	if err != nil {
		return nil, err
	}
	var collection RedfishCollection
	if err := json.Unmarshal(collectionBody, &collection); err != nil {
		return nil, fmt.Errorf("failed to decode collection from %s: %w", collectionURI, err)
	}
	for _, member := range collection.Members {
		memberURI := strings.TrimPrefix(member.ODataID, "/redfish/v1")
		memberBody, err := c.Get(memberURI)
		if err != nil {
			fmt.Printf("Warning: Failed to get member %s: %v\n", member.ODataID, err)
			continue
		}
		component := reflect.New(reflect.TypeOf(componentTypeExample).Elem()).Interface()
		if err := json.Unmarshal(memberBody, &component); err != nil {
			fmt.Printf("Warning: Failed to unmarshal component %s: %v\n", member.ODataID, err)
			continue
		}
		rfProps := reflect.ValueOf(component).Elem().Field(0).Interface().(CommonRedfishProperties)

		// Pass the parentSerial to mapCommonProperties
		specs = append(specs, mapCommonProperties(rfProps, deviceType, memberURI, parentURI, parentSerial))
	}
	return specs, nil
}

// mapCommonProperties maps Redfish fields to the API's DeviceSpec struct.
func mapCommonProperties(rfProps CommonRedfishProperties, deviceType, redfishURI, parentURI, parentSerial string) *device.DeviceSpec {
	partNum := rfProps.PartNumber
	if partNum == "" {
		partNum = rfProps.Model
	}
	uriBytes, _ := json.Marshal(redfishURI)
	parentURIBytes, _ := json.Marshal(parentURI)
	props := map[string]json.RawMessage{
		"redfish_uri":        uriBytes,
		"redfish_parent_uri": parentURIBytes,
	}

	return &device.DeviceSpec{
		DeviceType:         deviceType,
		Manufacturer:       rfProps.Manufacturer,
		PartNumber:         partNum,
		SerialNumber:       rfProps.SerialNumber,
		Properties:         props,
		ParentSerialNumber: parentSerial,
	}
}