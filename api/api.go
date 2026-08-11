package api

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"wme/monitor"
	"wme/virsh"
)

type API struct {
	client  *virsh.Client
	monitor *monitor.Monitor
	upgrader websocket.Upgrader
}

func NewAPI() *API {
	return &API{
		client:  virsh.NewClient(""),
		monitor: monitor.NewMonitor(),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (a *API) StartupScan() {
	a.client.ScanAllVMIPs()
}

func (a *API) SetupRoutes() {
	http.HandleFunc("/api/host", a.handleHost)
	http.HandleFunc("/api/host/stats", a.handleHostStats)
	http.HandleFunc("/api/vms", a.handleVMs)
	http.HandleFunc("/api/vms/", a.handleVMDetail)
	http.HandleFunc("/api/vm/", a.handleVMAction)
	http.HandleFunc("/api/storage", a.handleStorage)
	http.HandleFunc("/api/storage/", a.handleStorageDetail)
	http.HandleFunc("/api/networks", a.handleNetworks)
	http.HandleFunc("/api/networks/", a.handleNetworkDetail)
	http.HandleFunc("/api/isos", a.handleISOs)
	http.HandleFunc("/api/isos/upload", a.handleISOUpload)
	http.HandleFunc("/api/ws", a.handleWebSocket)
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func (a *API) handleHost(w http.ResponseWriter, r *http.Request) {
	info, err := a.client.GetHostInfo()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (a *API) handleHostStats(w http.ResponseWriter, r *http.Request) {
	stats, err := a.monitor.GetHostStats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (a *API) handleVMs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		vms, err := a.client.ListVMs()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		for i := range vms {
			if vms[i].State == "running" {
				if ips, err := a.client.GetVMIPs(vms[i].Name); err == nil {
					vms[i].IPs = ips
				}
			}
		}
		writeJSON(w, http.StatusOK, vms)
	case http.MethodPost:
		var req CreateVMRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		_, steps, err := a.buildVMConfig(req)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if req.Start {
			info, err := a.client.GetVMInfo(req.Name)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			if info.State != "running" {
				if err := a.client.StartVM(req.Name); err != nil {
					writeError(w, http.StatusInternalServerError, err)
					return
				}
			}
		}
		writeJSON(w, http.StatusCreated, map[string]interface{}{"status": "created", "name": req.Name, "steps": steps})
	default:
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
	}
}

func (a *API) handleVMDetail(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/vms/")
	if name == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("missing VM name"))
		return
	}

	info, err := a.client.GetVMInfo(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	if info.State == "running" {
		if ips, err := a.client.GetVMIPs(name); err == nil {
			info.IPs = ips
		}
	}

	disks, err := a.client.GetVMDisks(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	nics, err := a.client.GetVMNICs(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	vnc, _ := a.client.GetVNCInfo(name)
	stats, _ := a.monitor.GetVMStats(name)

	detail := VMDetail{
		Info:  info,
		Disks: disks,
		NICs:  nics,
		VNC:   vnc,
		Stats: stats,
	}
	writeJSON(w, http.StatusOK, detail)
}

func (a *API) handleVMAction(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/vm/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid action path"))
		return
	}
	name := parts[0]
	action := parts[1]

	var err error
	switch action {
	case "start":
		err = a.client.StartVM(name)
	case "shutdown":
		err = a.client.ShutdownVM(name)
	case "reboot":
		err = a.client.RebootVM(name)
	case "stop":
		err = a.client.ForceStopVM(name)
	case "suspend":
		err = a.client.SuspendVM(name)
	case "resume":
		err = a.client.ResumeVM(name)
	case "delete":
		err = a.client.DeleteVM(name)
	case "autostart":
		var req AutostartRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		err = a.client.SetAutostart(name, req.Enabled)
	default:
		writeError(w, http.StatusBadRequest, fmt.Errorf("unknown action: %s", action))
		return
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "action": action, "name": name})
}

func (a *API) handleStorage(w http.ResponseWriter, r *http.Request) {
	pools, err := a.client.GetStoragePools()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, pools)
}

func (a *API) handleStorageDetail(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/storage/")
	if name == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("missing pool name"))
		return
	}
	info, err := a.client.GetStoragePoolInfo(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (a *API) handleNetworks(w http.ResponseWriter, r *http.Request) {
	nets, err := a.client.GetNetworkList()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, nets)
}

func (a *API) handleNetworkDetail(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/networks/")
	if name == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("missing network name"))
		return
	}
	info, err := a.client.GetNetworkInfo(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

func (a *API) handleISOs(w http.ResponseWriter, r *http.Request) {
	isos, err := a.client.ListISOs()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, isos)
}

func (a *API) handleISOUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	defer file.Close()

	if !strings.HasSuffix(strings.ToLower(header.Filename), ".iso") {
		writeError(w, http.StatusBadRequest, fmt.Errorf("only .iso files are allowed"))
		return
	}

	destDir := "/var/lib/libvirt/images/isos"
	destPath := filepath.Join(destDir, filepath.Base(header.Filename))

	dst, err := os.Create(destPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"status": "uploaded",
		"name":   header.Filename,
		"path":   destPath,
	})
}

func (a *API) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := a.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			hostStats, err := a.monitor.GetHostStats()
			if err != nil {
				continue
			}
			vms, err := a.client.ListVMs()
			if err != nil {
				continue
			}
			var vmStats []monitor.VMStats
			for _, vm := range vms {
				if vm.State == "running" {
					stats, err := a.monitor.GetVMStats(vm.Name)
					if err == nil {
						vmStats = append(vmStats, stats)
					}
				}
			}
			msg := WSMessage{
				Type:      "stats",
				HostStats: hostStats,
				VMStats:   vmStats,
				Timestamp: time.Now(),
			}
			if err := conn.WriteJSON(msg); err != nil {
				return
			}
		}
	}
}

const (
	ubuntuCloudImageURL  = "https://cloud-images.ubuntu.com/releases/22.04/release/ubuntu-22.04-server-cloudimg-amd64.img"
	ubuntuCloudImagePath = "/var/lib/libvirt/images/base/ubuntu-22.04-server-cloudimg-amd64.img"
)

func generateSecurePassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = charset[int(b[i])%len(charset)]
	}
	return string(b), nil
}

func (a *API) validateKVM() error {
	if _, err := os.Stat("/dev/kvm"); os.IsNotExist(err) {
		return fmt.Errorf("KVM unavailable: /dev/kvm does not exist. Enable virtualization in BIOS/UEFI and ensure KVM module is loaded")
	}
	f, err := os.Open("/dev/kvm")
	if err != nil {
		return fmt.Errorf("KVM unavailable: cannot access /dev/kvm: %v. Ensure your user is in the kvm group", err)
	}
	f.Close()
	return nil
}

func (a *API) validateBridge(name string) error {
	if name == "" {
		return fmt.Errorf("network name is empty")
	}
	if name == "default" {
		return fmt.Errorf("network 'default' is not allowed. Use 'br0' for bridged networking")
	}
	out, err := exec.Command("bridge", "show").Output()
	if err == nil {
		if strings.Contains(string(out), name) {
			return nil
		}
		return fmt.Errorf("bridge '%s' not found", name)
	}
	out, err = exec.Command("ip", "link", "show", name).Output()
	if err == nil && strings.Contains(string(out), "state") {
		return nil
	}
	out, err = exec.Command("virsh", "net-list", "--all").Output()
	if err == nil && strings.Contains(string(out), name) {
		return nil
	}
	return fmt.Errorf("network '%s' not found", name)
}

func (a *API) ensureUbuntuCloudImage() error {
	if _, err := os.Stat(ubuntuCloudImagePath); err == nil {
		return nil
	}
	if err := os.MkdirAll("/var/lib/libvirt/images/base", 0755); err != nil {
		return fmt.Errorf("failed to create base image directory: %v", err)
	}
	resp, err := http.Get(ubuntuCloudImageURL)
	if err != nil {
		return fmt.Errorf("failed to download Ubuntu cloud image: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download Ubuntu cloud image: HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(ubuntuCloudImagePath)
	if err != nil {
		return fmt.Errorf("failed to create base image file: %v", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("failed to write base image: %v", err)
	}
	return nil
}

func (a *API) generateCloudInit(name, password, username string) (string, error) {
	if username == "" {
		username = "user"
	}
	if password == "" {
		pw, err := generateSecurePassword(16)
		if err != nil {
			return "", fmt.Errorf("failed to generate password: %v", err)
		}
		password = pw
	}
	tmpDir, err := os.MkdirTemp("", "cloudinit")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	userData := fmt.Sprintf(`#cloud-config
user: %s
password: %s
chpasswd: { expire: False }
ssh_pwauth: True
disable_root: false
hostname: %s
manage_etc_hosts: true
packages:
  - openssh-server
runcmd:
  - apt-get update
  - apt-get install -y qemu-guest-agent
  - systemctl start qemu-guest-agent
  - systemctl enable qemu-guest-agent
  - ifconfig > /root/ifconfig.txt
`, username, password, name)

	metaData := fmt.Sprintf(`instance-id: %s
local-hostname: %s
`, name, name)

	if err := os.WriteFile(filepath.Join(tmpDir, "user-data"), []byte(userData), 0644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "meta-data"), []byte(metaData), 0644); err != nil {
		return "", err
	}

	isoPath := filepath.Join("/var/lib/libvirt/images/isos", fmt.Sprintf("%s-cloud-init.iso", name))
	if err := os.MkdirAll("/var/lib/libvirt/images/isos", 0755); err != nil {
		return "", err
	}
	cmd := exec.Command("genisoimage", "-output", isoPath, "-volid", "cidata", "-joliet", "-rock", tmpDir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("genisoimage: %v: %s", err, stderr.String())
	}
	return isoPath, nil
}

func (a *API) createVMOverlay(baseImage, diskPath string, sizeGB int) error {
	if _, err := os.Stat(diskPath); err == nil {
		return fmt.Errorf("disk already exists: %s", diskPath)
	}
	cmd := exec.Command("qemu-img", "create", "-f", "qcow2", "-b", baseImage, "-F", "qcow2", diskPath, fmt.Sprintf("%dG", sizeGB))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("qemu-img overlay: %v: %s", err, stderr.String())
	}
	return nil
}

func (a *API) buildVMConfig(req CreateVMRequest) (string, []string, error) {
	var steps []string
	kvmAvailable := true
	if req.Name == "" {
		return "", nil, fmt.Errorf("name is required")
	}
	if !regexp.MustCompile(`^[a-zA-Z0-9_\-\.]+$`).MatchString(req.Name) {
		return "", nil, fmt.Errorf("invalid name: only letters, numbers, underscores, dashes and dots allowed")
	}
	if req.Password != "" && len(req.Password) < 8 {
		return "", nil, fmt.Errorf("password must be at least 8 characters")
	}
	if req.Memory <= 0 {
		req.Memory = 2048
	}
	if req.VCPUs <= 0 {
		req.VCPUs = 2
	}
	if req.DiskSize <= 0 {
		req.DiskSize = 20
	}
	if req.OS == "" {
		req.OS = "ubuntu"
	}
	if req.Network == "" {
		req.Network = "br0"
	}

	if req.Network == "default" {
		return "", nil, fmt.Errorf("network 'default' is not allowed. Use 'br0' for bridged networking")
	}
	if err := a.validateBridge(req.Network); err != nil {
		return "", nil, err
	}
	steps = append(steps, fmt.Sprintf("Validated network bridge: %s", req.Network))

	if err := a.validateKVM(); err != nil {
		kvmAvailable = false
		steps = append(steps, fmt.Sprintf("KVM unavailable: %s", err))
		steps = append(steps, "Falling back to software emulation (TCG)")
	} else {
		steps = append(steps, "Validated KVM acceleration")
	}

	if err := a.ensureUbuntuCloudImage(); err != nil {
		return "", nil, err
	}
	steps = append(steps, fmt.Sprintf("Using base image: %s", ubuntuCloudImagePath))

	diskPath := fmt.Sprintf("/var/lib/libvirt/images/%s.qcow2", req.Name)
	if err := a.createVMOverlay(ubuntuCloudImagePath, diskPath, req.DiskSize); err != nil {
		return "", nil, err
	}
	steps = append(steps, fmt.Sprintf("Created overlay disk: %s (%dGB, qcow2, backing=%s)", diskPath, req.DiskSize, ubuntuCloudImagePath))

	cloudInitPath, err := a.generateCloudInit(req.Name, req.Password, "user")
	if err != nil {
		return "", nil, fmt.Errorf("cloud-init generation failed: %v", err)
	}
	steps = append(steps, fmt.Sprintf("Generated cloud-init ISO: %s", cloudInitPath))

	args := []string{
		"--connect", "qemu:///system",
		"--name", req.Name,
		"--memory", fmt.Sprintf("%d", req.Memory),
		"--vcpus", fmt.Sprintf("%d", req.VCPUs),
		"--disk", fmt.Sprintf("%s,format=qcow2,bus=virtio", diskPath),
		"--disk", fmt.Sprintf("%s,format=raw,bus=sata", cloudInitPath),
		"--network", fmt.Sprintf("bridge=%s,model=virtio", req.Network),
		"--os-variant", "ubuntu22.04",
		"--graphics", "none",
		"--noautoconsole",
	}

	if !kvmAvailable {
		args = append(args, "--virt-type", "qemu")
	}

	if req.ISO != "" {
		args = append(args, "--cdrom", req.ISO)
		args = append(args, "--boot", "cdrom")
		steps = append(steps, fmt.Sprintf("Installation media: %s", req.ISO))
	} else {
		args = append(args, "--import")
		steps = append(steps, "Boot mode: import existing disk image")
	}

	cmd := exec.Command("virt-install", args...)
	cmd.Dir = "/var/lib/libvirt/images"
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", nil, fmt.Errorf("virt-install failed: %v: %s", err, stderr.String())
	}
	steps = append(steps, fmt.Sprintf("Defined VM with virt-install: name=%s, memory=%dMB, vcpus=%d, disk=%dGB, network=%s", req.Name, req.Memory, req.VCPUs, req.DiskSize, req.Network))

	return "", steps, nil
}

type CreateVMRequest struct {
	Name         string `json:"name"`
	Password     string `json:"password"`
	Memory       int    `json:"memory"`
	VCPUs        int    `json:"vcpus"`
	DiskSize     int    `json:"disk_size"`
	OS           string `json:"os"`
	Network      string `json:"network"`
	ISO          string `json:"iso"`
	Start        bool   `json:"start"`
}

type AutostartRequest struct {
	Enabled bool `json:"enabled"`
}

type VMDetail struct {
	Info  virsh.VMInfo    `json:"info"`
	Disks []virsh.Disk    `json:"disks"`
	NICs  []virsh.NIC     `json:"nics"`
	VNC   virsh.VNCInfo   `json:"vnc"`
	Stats monitor.VMStats `json:"stats"`
}

type WSMessage struct {
	Type      string             `json:"type"`
	HostStats monitor.HostStats  `json:"host_stats"`
	VMStats   []monitor.VMStats  `json:"vm_stats"`
	Timestamp time.Time          `json:"timestamp"`
}