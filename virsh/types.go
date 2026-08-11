package virsh

import "encoding/xml"

type VM struct {
	ID    int      `json:"id"`
	Name  string   `json:"name"`
	State string   `json:"state"`
	IPs   []string `json:"ips"`
}

type VMInfo struct {
	ID         int      `json:"id"`
	Name       string   `json:"name"`
	State      string   `json:"state"`
	VCPUs      int      `json:"vcpus"`
	MaxMemory  uint64   `json:"max_memory"`
	UsedMemory uint64   `json:"used_memory"`
	Persistent string   `json:"persistent"`
	Autostart  string   `json:"autostart"`
	IPs        []string `json:"ips"`
}

type Disk struct {
	Device string `json:"device"`
	Type   string `json:"type"`
	Target string `json:"target"`
	Source string `json:"source"`
	Size   uint64 `json:"size"`
}

type NIC struct {
	Type    string `json:"type"`
	MAC     string `json:"mac"`
	Source  string `json:"source"`
	Model   string `json:"model"`
	Network string `json:"network"`
}

type HostInfo struct {
	CPUModel     string  `json:"cpu_model"`
	CPUs         int     `json:"cpus"`
	CPUFrequency float64 `json:"cpu_frequency"`
	MemoryKB     uint64  `json:"memory_kb"`
}

type StoragePool struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Active bool   `json:"active"`
}

type StoragePoolInfo struct {
	Name       string `json:"name"`
	State      string `json:"state"`
	Capacity   uint64 `json:"capacity"`
	Allocation uint64 `json:"allocation"`
	Available  uint64 `json:"available"`
}

type Network struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Active bool   `json:"active"`
}

type NetworkInfo struct {
	Name    string `json:"name"`
	State   string `json:"state"`
	Bridge  string `json:"bridge"`
	Forward string `json:"forward"`
}

type VNCInfo struct {
	Type   string `json:"type"`
	Port   string `json:"port"`
	Listen string `json:"listen"`
}

type ISO struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Size uint64 `json:"size"`
}

type AgentNetworkResponse struct {
	Return []AgentInterface `json:"return"`
}

type AgentInterface struct {
	Name        string            `json:"name"`
	IPAddresses []AgentIPAddress  `json:"ip-addresses"`
}

type AgentIPAddress struct {
	IPAddress string `json:"ip-address"`
	Prefix    int    `json:"prefix"`
	Type      string `json:"type"`
}

type Domain struct {
	XMLName xml.Name `xml:"domain"`
	Devices Devices  `xml:"devices"`
}

type Devices struct {
	Disks      []DiskXML      `xml:"disk"`
	Interfaces []InterfaceXML `xml:"interface"`
	Graphics   []GraphicsXML  `xml:"graphics"`
}

type DiskXML struct {
	Device string   `xml:"device,attr"`
	Type   string   `xml:"type,attr"`
	Target DiskTgt  `xml:"target"`
	Source DiskSrc  `xml:"source"`
}

type DiskTgt struct {
	Dev string `xml:"dev,attr"`
}

type DiskSrc struct {
	File string `xml:"file,attr"`
}

type InterfaceXML struct {
	Type   string      `xml:"type,attr"`
	MAC    MACXML      `xml:"mac"`
	Source IfaceSource `xml:"source"`
	Model  IfaceModel  `xml:"model"`
}

type MACXML struct {
	Address string `xml:"address,attr"`
}

type IfaceSource struct {
	Bridge  string `xml:"bridge,attr"`
	Network string `xml:"network,attr"`
}

type IfaceModel struct {
	Type string `xml:"type,attr"`
}

type GraphicsXML struct {
	Type   string `xml:"type,attr"`
	Port   string `xml:"port,attr"`
	Listen string `xml:"listen,attr"`
}