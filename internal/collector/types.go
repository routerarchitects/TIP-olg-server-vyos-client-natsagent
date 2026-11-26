package collector

// StatePayload represents the OpenWiFi router telemetry payload published inside StateEnvelope.
type StatePayload struct {
	Version    int                    `json:"version"`
	Unit       UnitState              `json:"unit"`
	Interfaces []InterfaceState       `json:"interfaces"`
	LinkState  LinkStateReport        `json:"link-state"`
	LLDPPeers  LLDPPeersReport        `json:"lldp-peers"`
	Radios     []interface{}          `json:"radios"`
}

// UnitState carries system resource metrics.
type UnitState struct {
	Load      [3]float64  `json:"load"`
	CPULoad   []int       `json:"cpu_load"`
	LocalTime int64       `json:"localtime"`
	Memory    MemoryState `json:"memory"`
	Uptime    uint64      `json:"uptime"`
}

// MemoryState carries RAM statistics in bytes.
type MemoryState struct {
	Total    uint64 `json:"total"`
	Free     uint64 `json:"free"`
	Cached   uint64 `json:"cached"`
	Buffered uint64 `json:"buffered"`
}

// InterfaceState carries per-interface configuration, counters, deltas, and neighbors.
type InterfaceState struct {
	Name          string                 `json:"name"`
	Location      string                 `json:"location,omitempty"`
	Uptime        uint64                 `json:"uptime,omitempty"`
	IPv4          *InterfaceIPv4         `json:"ipv4,omitempty"`
	IPv6Addresses []InterfaceIPv6Address `json:"ipv6_addresses,omitempty"`
	Counters      InterfaceCounters      `json:"counters"`
	DeltaCounters InterfaceDeltaCounters `json:"delta_counters"`
	Clients       []InterfaceClient      `json:"clients,omitempty"`
}

// InterfaceIPv4 represents IPv4 network addressing on an interface.
type InterfaceIPv4 struct {
	Addresses  []string `json:"addresses,omitempty"`
	DHCPServer string   `json:"dhcp_server,omitempty"`
	LeaseTime  uint32   `json:"leasetime,omitempty"`
}

// InterfaceIPv6Address represents a schema-compliant IPv6 address entry with optional valid lifetime.
type InterfaceIPv6Address struct {
	Address string `json:"address"`
	Valid   uint32 `json:"valid,omitempty"`
}

// InterfaceCounters carries cumulative interface traffic and error statistics.
type InterfaceCounters struct {
	RxBytes    uint64 `json:"rx_bytes"`
	RxPackets  uint64 `json:"rx_packets"`
	TxBytes    uint64 `json:"tx_bytes"`
	TxPackets  uint64 `json:"tx_packets"`
	RxDropped  uint64 `json:"rx_dropped"`
	RxErrors   uint64 `json:"rx_errors"`
	TxDropped  uint64 `json:"tx_dropped"`
	TxErrors   uint64 `json:"tx_errors"`
	Multicast  uint64 `json:"multicast"`
	Collisions uint64 `json:"collisions"`
}

// InterfaceDeltaCounters carries traffic increment since the previous sampling tick.
type InterfaceDeltaCounters struct {
	RxBytes   uint64 `json:"rx_bytes"`
	RxPackets uint64 `json:"rx_packets"`
	TxBytes   uint64 `json:"tx_bytes"`
	TxPackets uint64 `json:"tx_packets"`
}

// InterfaceClient represents an observed L3 neighbor.
type InterfaceClient struct {
	MAC           string   `json:"mac"`
	IPv4Addresses []string `json:"ipv4_addresses,omitempty"`
	IPv6Addresses []string `json:"ipv6_addresses,omitempty"`
	Ports         []string `json:"ports,omitempty"`
	LastSeen      *uint32  `json:"last_seen,omitempty"`
}

// LinkStateReport groups physical interfaces into WAN (upstream) and LAN (downstream).
type LinkStateReport struct {
	Upstream   map[string]PortLinkState `json:"upstream"`
	Downstream map[string]PortLinkState `json:"downstream"`
}

// PortLinkState carries physical carrier, negotiated speed, and duplex status.
type PortLinkState struct {
	Carrier bool   `json:"carrier"`
	Speed   uint32 `json:"speed"`
	Duplex  string `json:"duplex,omitempty"`
}

// LLDPPeersReport groups LLDP neighbor maps under assigned physical port roles.
type LLDPPeersReport struct {
	Upstream   map[string]interface{} `json:"upstream"`
	Downstream map[string]interface{} `json:"downstream"`
}
