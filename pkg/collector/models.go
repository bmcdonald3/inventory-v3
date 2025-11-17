package collector

import (
	"net/http"

	"github.com/example/inventory-v3/pkg/resources/device"
)

// --- Redfish Client Struct ---

// RedfishClient holds connection details and the HTTP client instance.
type RedfishClient struct {
	BaseURL    string
	Username   string
	Password   string
	HTTPClient *http.Client
}

// --- Redfish Helper Structs ---
// These are used for unmarshaling Redfish JSON

// SystemInventory holds the discovered devices related to one System/Node.
// It now holds the canonical DeviceSpec structs.
type SystemInventory struct {
	NodeSpec *device.DeviceSpec
	CPUs     []*device.DeviceSpec
	DIMMs    []*device.DeviceSpec
}

// RedfishCollection defines the structure for Redfish collection responses.
type RedfishCollection struct {
	Members []struct {
		ODataID string `json:"@odata.id"`
	} `json:"Members"`
}

// CommonRedfishProperties contains the fields required by the Device model.
type CommonRedfishProperties struct {
	Manufacturer string `json:"Manufacturer,omitempty"`
	Model        string `json:"Model,omitempty"`
	PartNumber   string `json:"PartNumber,omitempty"`
	SerialNumber string `json:"SerialNumber,omitempty"`
}

// RedfishSystem defines the structure for a System resource (the Node).
type RedfishSystem struct {
	CommonRedfishProperties // Embeds the common fields
	Processors              struct {
		ODataID string `json:"@odata.id"`
	} `json:"Processors"`
	Memory struct {
		ODataID string `json:"@odata.id"`
	} `json:"Memory"`
}

// RedfishProcessor defines the structure for a Processor resource (the CPU).
type RedfishProcessor struct {
	CommonRedfishProperties // Embeds the common fields
}

// RedfishMemory defines the structure for a Memory resource (the DIMM).
type RedfishMemory struct {
	CommonRedfishProperties // Embeds the common fields
}