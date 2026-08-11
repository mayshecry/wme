# WME Virtualization Manager

A web-based virtualization management interface for [libvirt](https://libvirt.org/) / [virsh](https://manpages.ubuntu.com/manpages/noble/en/man1/virsh.1.html). Built with Go on the backend and a dark-theme SPA frontend, WME provides real-time monitoring, full VM lifecycle management, storage & network browsing, and ISO management — all through an intuitive web UI.

## Features

- **Virtual Machine Management** — List, create, start, shutdown, reboot, suspend, resume, force-stop, and delete VMs through a responsive web interface
- **Real-Time Monitoring** — Live host and per-VM stats (CPU, memory, disk, network) streamed over WebSocket, with historical performance charts
- **VM Creation Wizard** — Guided 4-step wizard that leverages Ubuntu cloud images, cloud-init, and `virt-install` to provision new VMs with custom CPU, memory, disk, network, and installation ISO
- **Storage Pools** — Browse and inspect libvirt storage pools and their capacity/allocation/availability
- **Networks** — View and inspect libvirt virtual networks and their bridge/forwarding configuration
- **ISO Management** — List available ISO images and upload new ones with progress tracking
- **KVM Offload Awareness** — Automatically detects KVM availability and falls back to TCG software emulation if `/dev/kvm` is inaccessible

## Screenshots

### Dashboard
![Dashboard](static/screenshot-dashboard.png)

### Virtual Machines
![VMs](static/screenshot-vms.png)

> _Screenshots are illustrative. Place actual screenshots in the `static/` directory and update the paths above._

## Requirements

### System
- A Linux host with **libvirt** and **QEMU/KVM** installed and running
- The `virsh`, `qemu-img`, `virt-install`, and `genisoimage` CLI tools available on `PATH`
- Optionally KVM hardware acceleration (`/dev/kvm` accessible by your user)

### External Commands Used
| Command | Purpose |
|---|---|
| `virsh` | All libvirt domain, storage, and network management |
| `qemu-img` | Overlay disk creation for new VMs |
| `virt-install` | VM definition and boot |
| `genisoimage` | Cloud-init ISO generation |
| `bridge` / `ip` / `virsh net-list` | Network bridge validation |

## Installation

1. **Clone the repository:**

   ```bash
   git clone https://github.com/yourusername/wme.git
   cd wme
   ```

2. **Build the binary:**

   ```bash
   go build -o wme .
   ```

3. **(Optional) Install as a system service:**

   Copy the binary to a location on your `PATH` and create a systemd unit:

   ```ini
   [Unit]
   Description=WME Virtualization Manager
   After=libvirtd.service

   [Service]
   ExecStart=/usr/local/bin/wme
   Restart=always
   User=libvirt

   [Install]
   WantedBy=multi-user.target
   ```

## Configuration

WME is configured via environment variables:

| Variable | Default | Description |
|---|---|---|
| `PORT` | `30120` | The TCP port the HTTP server listens on |

No other configuration file is required — WME connects to the local libvirt daemon via `qemu:///system`.

## Usage

### Running the Server

```bash
./wme
```

Or with a custom port:

```bash
PORT=8080 ./wme
```

Once running, open `http://localhost:30120` in your browser.

### Directory Layout

```
wme/
├── main.go                  # Application entrypoint — serves static files and the API
├── go.mod                   # Go module definition (module: wme)
├── api/
│   └── api.go               # HTTP REST API + WebSocket handlers
├── monitor/
│   └── monitor.go           # Host & VM performance monitor (/proc parsing + virsh stats)
├── virsh/
│   ├── client.go            # virsh command wrapper (VM/storage/network lifecycle)
│   └── types.go             # Type definitions for virsh entities & XML parsing
└── static/                  # Frontend assets
    ├── index.html           # Main SPA page
    ├── css/
    │   └── style.css        # Dark theme stylesheet
    └── js/
        └── app.js           # Frontend logic (vanilla JS + Chart.js)
```

## API

All endpoints return JSON. The WebSocket endpoint streams real-time stats.

### REST Endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/host` | Host hardware info (CPU model, core count, frequency, memory) |
| `GET` | `/api/host/stats` | Current host performance snapshot (CPU, memory, disk, network, load, uptime) |
| `GET` | `/api/vms` | List all VMs with state and IP addresses |
| `POST` | `/api/vms` | Create a new VM (see **Create VM Request** below) |
| `GET` | `/api/vms/{name}` | Detailed VM info (disks, NICs, VNC, live stats) |
| `POST` | `/api/vm/{name}/{action}` | Perform an action on a VM — `start`, `shutdown`, `reboot`, `stop`, `suspend`, `resume` |
| `DELETE` | `/api/vm/{name}/delete` | *(via POST with `delete` action)* Delete a VM and its storage |
| `GET` | `/api/storage` | List storage pools |
| `GET` | `/api/storage/{name}` | Storage pool details (capacity, allocation, available) |
| `GET` | `/api/networks` | List virtual networks |
| `GET` | `/api/networks/{name}` | Network details (bridge, forward mode) |
| `GET` | `/api/isos` | List available ISO images |
| `POST` | `/api/isos/upload` | Upload an ISO file (multipart form, `.iso` only) |
| `GET` | `/api/ws` | WebSocket — streams real-time host & VM stats every 2 seconds |

### Create VM Request

`POST /api/vms`

```json
{
  "name": "my-vm",
  "password": "secret123",
  "memory": 2048,
  "vcpus": 2,
  "disk_size": 20,
  "os": "ubuntu",
  "network": "br0",
  "iso": "/var/lib/libvirt/images/isos/ubuntu-22.04.iso",
  "start": true
}
```

| Field | Type | Default | Description |
|---|---|---|---|
| `name` | string | — | VM name (alphanumeric, `_`, `-`, `.` only) |
| `password` | string | auto-generated | Root password (min 8 chars if provided) |
| `memory` | int | `2048` | Memory in MB |
| `vcpus` | int | `2` | Number of vCPUs |
| `disk_size` | int | `20` | Disk size in GB |
| `os` | string | `ubuntu` | OS variant |
| `network` | string | `br0` | Bridge network name (must exist; `default` not allowed) |
| `iso` | string | `""` | Path to installation ISO (optional) |
| `start` | bool | `false` | Whether to start the VM after creation |

### WebSocket Message Format

```json
{
  "type": "stats",
  "host_stats": {
    "cpu_usage": 12.5,
    "memory_total": 16777216000,
    "memory_used": 8388608000,
    "memory_free": 8388608000,
    "memory_usage": 50.0,
    "disk_total": 107374182400,
    "disk_used": 53687091200,
    "disk_free": 53687091200,
    "disk_usage": 50.0,
    "net_rx": 1024000,
    "net_tx": 512000,
    "net_rx_rate": 100.5,
    "net_tx_rate": 50.2,
    "load_avg": [0.1, 0.2, 0.15],
    "uptime": 3600,
    "processes": 150,
    "timestamp": "2024-01-15T10:30:00Z"
  },
  "vm_stats": [
    {
      "name": "my-vm",
      "cpu_usage": 5.2,
      "memory_used": 1073741824,
      "memory_max": 2147483648,
      "net_rx": 1024,
      "net_tx": 512,
      "disk_read": 100,
      "disk_write": 200,
      "timestamp": "2024-01-15T10:30:00Z"
    }
  ],
  "timestamp": "2024-01-15T10:30:00Z"
}
```

## Architecture

WME is structured into four layers:

```
                    ┌─────────────────────────────────────────┐
                    │              Static Frontend            │
                    │  (HTML / CSS / JS — Chart.js)            │
                    └──────────────────┬──────────────────────┘
                                       │ HTTP / WebSocket
                                       ▼
                    ┌─────────────────────────────────────────┐
                    │         API Server (api/api.go)         │
                    │  REST endpoints + WebSocket real-time   │
                    └──────┬───────────────────────┬──────────┘
                           │                       │
              ┌────────────▼───────┐   ┌───────────▼──────────┐
              │   virsh Client     │   │     Monitor          │
              │  (virsh/client.go) │   │  (monitor/monitor.go)│
              │  VM/storage/net mgmt│   │ /proc + virsh stats  │
              └────────────────────┘   └──────────────────────┘
                           │
                           ▼
              ┌─────────────────────────────────┐
              │        libvirt / QEMU            │
              │  (virsh, virt-install, qemu-img) │
              └─────────────────────────────────┘
```

- **`api/api.go`** — The HTTP layer. Serves the static frontend, exposes REST endpoints for VM/storage/network management, handles VM creation (including cloud-init ISO generation and `virt-install` orchestration), and runs the `/api/ws` WebSocket that pushes real-time stats.
- **`virsh/client.go`** — A thin wrapper around the `virsh` CLI. All VM lifecycle, storage, and network operations are delegated here.
- **`monitor/monitor.go`** — Collects host performance data by parsing `/proc/stat`, `/proc/meminfo`, `/proc/net/dev`, `df`, `/proc/loadavg`, `/proc/uptime`, and `/proc` process counts. VM stats come from `virsh domstats` and `virsh dommemstat`.
- **`static/`** — A single-page application with a dark theme. Uses vanilla JavaScript with [Chart.js](https://www.chartjs.org/) for live performance graphs and a WebSocket connection for real-time updates.

## Building

```bash
# Download dependencies
go mod tidy

# Build
go build -o wme .

# Run
./wme
```

## Contributing

Contributions are welcome! Please feel free to submit a pull request.

## License

See the [LICENSE](LICENSE) file for details.
