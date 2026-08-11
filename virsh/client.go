package virsh

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Client struct {
	URI string
}

func NewClient(uri string) *Client {
	if uri == "" {
		uri = "qemu:///system"
	}
	return &Client{URI: uri}
}

func (c *Client) run(args ...string) (string, error) {
	cmdArgs := []string{"-c", c.URI}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.Command("virsh", cmdArgs...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("virsh %s: %v: %s", strings.Join(args, " "), err, stderr.String())
	}
	return strings.TrimSpace(stdout.String()), nil
}

func (c *Client) ListVMs() ([]VM, error) {
	out, err := c.run("list", "--all")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(out, "\n")
	var vms []VM
	for i, line := range lines {
		if i < 2 || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		id := fields[0]
		name := fields[1]
		state := fields[2]
		vmID := 0
		if id != "-" {
			vmID, _ = strconv.Atoi(id)
		}
		vms = append(vms, VM{
			ID:    vmID,
			Name:  name,
			State: state,
		})
	}
	return vms, nil
}

func (c *Client) GetVMInfo(name string) (VMInfo, error) {
	out, err := c.run("dominfo", name)
	if err != nil {
		return VMInfo{}, err
	}
	info := VMInfo{Name: name}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "Id":
			info.ID, _ = strconv.Atoi(val)
		case "State":
			info.State = val
		case "CPU(s)":
			info.VCPUs, _ = strconv.Atoi(val)
		case "Max memory":
			info.MaxMemory, _ = strconv.ParseUint(val, 10, 64)
		case "Used memory":
			info.UsedMemory, _ = strconv.ParseUint(val, 10, 64)
		case "Persistent":
			info.Persistent = val
		case "Autostart":
			info.Autostart = val
		}
	}
	return info, nil
}

func (c *Client) GetVMDomXML(name string) (string, error) {
	return c.run("dumpxml", name)
}

func (c *Client) GetVMDisks(name string) ([]Disk, error) {
	xmlStr, err := c.GetVMDomXML(name)
	if err != nil {
		return nil, err
	}
	var dom Domain
	if err := xml.Unmarshal([]byte(xmlStr), &dom); err != nil {
		return nil, err
	}
	var disks []Disk
	for _, disk := range dom.Devices.Disks {
		disks = append(disks, Disk{
			Device: disk.Device,
			Type:   disk.Type,
			Target: disk.Target.Dev,
			Source: disk.Source.File,
			Size:   0,
		})
	}
	return disks, nil
}

func (c *Client) GetVMNICs(name string) ([]NIC, error) {
	xmlStr, err := c.GetVMDomXML(name)
	if err != nil {
		return nil, err
	}
	var dom Domain
	if err := xml.Unmarshal([]byte(xmlStr), &dom); err != nil {
		return nil, err
	}
	var nics []NIC
	for _, iface := range dom.Devices.Interfaces {
		nics = append(nics, NIC{
			Type:    iface.Type,
			MAC:     iface.MAC.Address,
			Source:  iface.Source.Bridge,
			Model:   iface.Model.Type,
			Network: iface.Source.Network,
		})
	}
	return nics, nil
}

func (c *Client) StartVM(name string) error {
	_, err := c.run("start", name)
	return err
}

func (c *Client) ShutdownVM(name string) error {
	_, err := c.run("shutdown", name)
	return err
}

func (c *Client) RebootVM(name string) error {
	_, err := c.run("reboot", name)
	return err
}

func (c *Client) ForceStopVM(name string) error {
	_, err := c.run("destroy", name)
	return err
}

func (c *Client) SuspendVM(name string) error {
	_, err := c.run("suspend", name)
	return err
}

func (c *Client) ResumeVM(name string) error {
	_, err := c.run("resume", name)
	return err
}

func (c *Client) DeleteVM(name string) error {
	info, err := c.GetVMInfo(name)
	if err != nil {
		return err
	}
	if info.State == "running" || info.State == "paused" || info.State == "blocked" || info.State == "crashed" {
		if _, err := c.run("destroy", name); err != nil {
			return err
		}
	}
	_, err = c.run("undefine", name, "--remove-all-storage", "--managed-save", "--snapshots-metadata")
	if err != nil {
		return err
	}

	diskPath := fmt.Sprintf("/var/lib/libvirt/images/%s.qcow2", name)
	os.Remove(diskPath)

	cloudInitPath := fmt.Sprintf("/var/lib/libvirt/images/isos/%s-cloud-init.iso", name)
	os.Remove(cloudInitPath)

	return nil
}

func (c *Client) SetAutostart(name string, enabled bool) error {
	flag := "--disable"
	if enabled {
		flag = "--enable"
	}
	_, err := c.run("autostart", name, flag)
	return err
}

func (c *Client) CreateVM(config string) error {
	cmd := exec.Command("virsh", "-c", c.URI, "define", "/dev/stdin")
	cmd.Stdin = strings.NewReader(config)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("define: %v: %s", err, stderr.String())
	}
	return nil
}

func (c *Client) GetHostInfo() (HostInfo, error) {
	info := HostInfo{}
	out, err := c.run("nodeinfo")
	if err != nil {
		return info, err
	}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "CPU model":
			info.CPUModel = val
		case "CPU(s)":
			info.CPUs, _ = strconv.Atoi(val)
		case "CPU frequency":
			info.CPUFrequency, _ = strconv.ParseFloat(strings.Fields(val)[0], 64)
		case "Memory size":
			info.MemoryKB, _ = strconv.ParseUint(val, 10, 64)
		}
	}
	return info, nil
}

func (c *Client) GetStoragePools() ([]StoragePool, error) {
	out, err := c.run("pool-list", "--all")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(out, "\n")
	var pools []StoragePool
	for i, line := range lines {
		if i < 2 || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pools = append(pools, StoragePool{
			Name:   fields[0],
			State:  fields[1],
			Active: fields[1] == "active",
		})
	}
	return pools, nil
}

func (c *Client) GetStoragePoolInfo(name string) (StoragePoolInfo, error) {
	info := StoragePoolInfo{Name: name}
	out, err := c.run("pool-info", name)
	if err != nil {
		return info, err
	}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "State":
			info.State = val
		case "Capacity":
			info.Capacity, _ = strconv.ParseUint(strings.Fields(val)[0], 10, 64)
		case "Allocation":
			info.Allocation, _ = strconv.ParseUint(strings.Fields(val)[0], 10, 64)
		case "Available":
			info.Available, _ = strconv.ParseUint(strings.Fields(val)[0], 10, 64)
		}
	}
	return info, nil
}

func (c *Client) GetNetworkList() ([]Network, error) {
	out, err := c.run("net-list", "--all")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(out, "\n")
	var nets []Network
	for i, line := range lines {
		if i < 2 || strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		nets = append(nets, Network{
			Name:   fields[0],
			State:  fields[1],
			Active: fields[1] == "active",
		})
	}
	return nets, nil
}

func (c *Client) GetNetworkInfo(name string) (NetworkInfo, error) {
	info := NetworkInfo{Name: name}
	out, err := c.run("net-info", name)
	if err != nil {
		return info, err
	}
	for _, line := range strings.Split(out, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "State":
			info.State = val
		case "Bridge":
			info.Bridge = val
		case "Forward":
			info.Forward = val
		}
	}
	return info, nil
}

func (c *Client) GetVNCInfo(name string) (VNCInfo, error) {
	xmlStr, err := c.GetVMDomXML(name)
	if err != nil {
		return VNCInfo{}, err
	}
	var dom Domain
	if err := xml.Unmarshal([]byte(xmlStr), &dom); err != nil {
		return VNCInfo{}, err
	}
	for _, graphics := range dom.Devices.Graphics {
		if graphics.Type == "vnc" {
			return VNCInfo{
				Type:   graphics.Type,
				Port:   graphics.Port,
				Listen: graphics.Listen,
			}, nil
		}
	}
	return VNCInfo{}, nil
}

func (c *Client) GetVMIPs(name string) ([]string, error) {
	out, err := c.run("domifaddr", name, "--source", "agent")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(out, "\n")
	var ips []string
	seen := make(map[string]bool)
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		if !strings.Contains(fields[2], "br0") {
			continue
		}
		ipField := fields[len(fields)-1]
		var ip string
		if strings.Contains(ipField, "/") {
			ip = strings.Split(ipField, "/")[0]
		} else {
			ip = ipField
		}
		if ip != "" && !seen[ip] {
			seen[ip] = true
			ips = append(ips, ip)
		}
	}
	return ips, nil
}

func (c *Client) ScanAllVMIPs() {
	vms, err := c.ListVMs()
	if err != nil {
		return
	}
	for _, vm := range vms {
		if vm.State == "running" {
			c.GetVMIPs(vm.Name)
		}
	}
}

func (c *Client) GenerateCloudInitISO(name, rootPassword string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "cloudinit")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	userData := fmt.Sprintf(`#cloud-config
password: %s
chpasswd: { expire: False }
ssh_pwauth: True
disable_root: false
hostname: %s
manage_etc_hosts: true
runcmd:
  - apt-get update
  - apt-get install -y qemu-guest-agent
  - systemctl start qemu-guest-agent
  - systemctl enable qemu-guest-agent
  - ifconfig > /root/ifconfig.txt
`, rootPassword, name)

	metaData := fmt.Sprintf(`instance-id: %s
local-hostname: %s
`, name, name)

	if err := os.WriteFile(filepath.Join(tmpDir, "user-data"), []byte(userData), 0644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "meta-data"), []byte(metaData), 0644); err != nil {
		return "", err
	}

	destDir := "/var/lib/libvirt/images"
	destPath := filepath.Join(destDir, fmt.Sprintf("%s-cloud-init.iso", name))

	cmd := exec.Command("genisoimage", "-output", destPath, "-volid", "cidata", "-joliet", "-rock", tmpDir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("genisoimage: %v: %s", err, stderr.String())
	}

	return destPath, nil
}

func (c *Client) ListISOs() ([]ISO, error) {
	dirs := []string{
		"/var/lib/libvirt/images/isos",
		"/var/lib/libvirt/images",
	}
	seen := make(map[string]bool)
	var isos []ISO
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !strings.HasSuffix(strings.ToLower(name), ".iso") {
				continue
			}
			path := filepath.Join(dir, name)
			if seen[path] {
				continue
			}
			seen[path] = true
			info, err := entry.Info()
			if err != nil {
				continue
			}
			isos = append(isos, ISO{
				Name: name,
				Path: path,
				Size: uint64(info.Size()),
			})
		}
	}
	return isos, nil
}
