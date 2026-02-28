# QemuRun-pve - Documentation

> Back to [README](../README.md)

## Prerequisites

- A Proxmox VE cluster, with the service running as `root` on the **main node** (needs `qm`, `pvesh`, `pvesm`, and read access to `/etc/pve/nodes`)
- Go 1.25 or higher (only when building from source)
- Passwordless key-based `root` SSH from the main node to every other node
- One of `id_ed25519.pub` / `id_rsa.pub` / `id_ecdsa.pub` under `~/.ssh/` of the service user (the install flow generates an ed25519 key with `ssh-keygen` when none exists)
- A `/24` subnet where each VM's last IP octet equals its VMID (e.g. VMID `120` → `192.168.0.120`), so VMIDs must fall within `100`–`254`
- An active storage pool of type `dir` / `zfspool` / `lvmthin` / `nfs` that supports a `cloudinit` disk
- Outbound access from the main node to the official cloud image mirrors (Debian, Rocky Linux, Ubuntu)

## Installation

### From Source (Recommended)

The service resolves `.env`, the `sh/` init scripts, and the `.go_qemu_*` state files relative to its working directory, so run it inside the cloned repository.

```bash
git clone https://github.com/pardnchiu/QemuRun-pve.git
cd QemuRun-pve
cp .env.example .env
touch .go_qemu_disabled
go build -o QemuRun-pve ./cmd/api
./QemuRun-pve
```

On startup the server prints every registered route followed by `goQemu run at localhost:<PORT>`.

### From a Release Binary

Pushing a `v*` tag triggers GitHub Actions to build a `linux/amd64` binary packed as `QemuRun-pve@<tag>.tar.gz`. The extracted `app` must sit in the same working directory as `sh/`, `.env`, and `.go_qemu_disabled`.

```bash
git clone https://github.com/pardnchiu/QemuRun-pve.git /opt/QemuRun-pve
cd /opt/QemuRun-pve
curl -fsSL -o release.tar.gz \
  "https://github.com/pardnchiu/QemuRun-pve/releases/download/<tag>/QemuRun-pve@<tag>.tar.gz"
tar -xzf release.tar.gz
cp .env.example .env
touch .go_qemu_disabled
./app
```

### Run as a systemd Service

```ini
[Unit]
Description=QemuRun-pve API
After=network-online.target

[Service]
User=root
WorkingDirectory=/opt/QemuRun-pve
ExecStart=/opt/QemuRun-pve/QemuRun-pve
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

```bash
systemctl daemon-reload
systemctl enable --now QemuRun-pve
```

## Configuration

### Environment Variables

`godotenv` loads `.env` from the working directory at startup; when the file is missing the server only logs a warning and falls back to the process environment.

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `PORT` | Yes | — | API listen port; startup fails when unset |
| `GATEWAY` | Yes | — | VM gateway such as `192.168.0.1`; its first three octets also form the VM IP prefix |
| `MAIN_NODE` | Yes | — | Name of the Proxmox node hosting the service |
| `NODE_<name>` | Yes | — | Management IP of every node, main node included, e.g. `NODE_pve2=192.168.0.12` |
| `ALLOW_IPS` | Yes | — | Comma-separated client IPs allowed to call mutating endpoints; `0.0.0.0` allows all; empty denies all |
| `ASSIGN_STORAGE` | Yes | — | Storage pool for imported disks and cloud-init, e.g. `local-zfs` |
| `ASSIGN_IP_START` | No | `100` | Lower bound for auto-assigned VMIDs; values below `100` are raised to `100` |
| `ASSIGN_IP_END` | No | `254` | Upper bound for auto-assigned VMIDs; values above `254` are lowered to `254` |
| `VM_MAX_CPU` | No | unlimited | vCPU cap at install time; larger requests are clamped |
| `VM_MAX_RAM` | No | unlimited | Memory cap at install time (MB) |
| `VM_MAX_DISK` | No | unlimited | Disk cap at install time (GB) |
| `VM_BALLOON_MIN` | No | disabled | Balloon reserve (MB); when set, memory ≥ this value + 1024 enables NUMA and ballooning |
| `VM_ROOT_PASSWORD` | No | script default | Root password passed to the OS init script; when empty the script's built-in default `0123456789` applies |

### Working Directory State Files

| File | Required | Description |
|------|----------|-------------|
| `.go_qemu_disabled` | Yes | Disabled list, one `<vmid>:<name>` per line; listed VMs cannot be controlled through the API. **Without this file `/api/vm/list` and every endpoint that checks VM state fail**, so create an empty file when nothing is disabled |
| `.go_qemu_pubkey_admin` | No | Extra admin SSH public keys injected into every new VM; multiple lines allowed |
| `.go_qemu_cpu_type` | Auto | Cached cluster CPU type; delete it after node hardware changes to re-detect |

### `.env` Example

```bash
PORT=8080
MAIN_NODE=pve1
GATEWAY=192.168.0.1
ALLOW_IPS=192.168.0.10,192.168.0.20

NODE_pve1=192.168.0.11
NODE_pve2=192.168.0.12
NODE_pve3=192.168.0.13

ASSIGN_IP_START=100
ASSIGN_IP_END=254
ASSIGN_STORAGE=local-zfs

VM_MAX_CPU=32
VM_MAX_DISK=64
VM_MAX_RAM=32768
VM_BALLOON_MIN=2048

VM_ROOT_PASSWORD=
```

## Usage

### Basic

```bash
# Health check
curl http://192.168.0.11:8080/api/health
# ok

# List every VM along with node utilization
curl http://192.168.0.11:8080/api/vm/list

# Query one VM's status (main-node VMs only)
curl http://192.168.0.11:8080/api/vm/120/status
# running
```

### Create a VM

`-N` disables curl buffering so SSE progress shows up live. Omitting `id` auto-assigns both VMID and IP.

```bash
curl -N -X POST http://192.168.0.11:8080/api/vm/install \
  -H "Content-Type: application/json" \
  -d '{
    "name": "web",
    "os": "debian",
    "version": "12",
    "cpu": 2,
    "ram": 4096,
    "disk": "32G",
    "pubkey": "ssh-ed25519 AAAA... user@laptop"
  }'
```

```text
data: {"step":"","status":"processing","message":"[*] start VM installation"}

data: {"step":"preparation > checking VMID","status":"success","message":"[+] auto-assigned VMID: 120 (0.84s)"}

data: {"step":"preparation > assigning IP","status":"success","message":"[+] assigned IP: 192.168.0.120/24 (0.00s)"}

...

data: {"step":"VM initialization > finalizing","status":"success","message":"[*] IP: 192.168.0.120"}

data: {"step":"VM initialization > finalizing","status":"success","message":"[*] User: debian"}

event: close
data: {}
```

### Lifecycle Operations

```bash
# Start (SSE, ends once SSH is reachable)
curl -N -X POST http://192.168.0.11:8080/api/vm/120/start

# Reboot (SSE)
curl -N -X POST http://192.168.0.11:8080/api/vm/120/reboot

# Graceful shutdown / force stop
curl -X POST http://192.168.0.11:8080/api/vm/120/shutdown
curl -X POST http://192.168.0.11:8080/api/vm/120/stop

# Destroy (VM must be stopped)
curl -X POST http://192.168.0.11:8080/api/vm/120/destroy
```

### Resize Resources

Every operation below requires the VM to be stopped.

```bash
# Set vCPU (1–32)
curl -X POST http://192.168.0.11:8080/api/vm/120/set/cpu \
  -H "Content-Type: application/json" -d '{"cpu": 4}'

# Set memory in MB (512–32768)
curl -X POST http://192.168.0.11:8080/api/vm/120/set/memory \
  -H "Content-Type: application/json" -d '{"memory": 8192}'

# Grow disk (increment)
curl -X POST http://192.168.0.11:8080/api/vm/120/set/disk \
  -H "Content-Type: application/json" -d '{"disk": "16G"}'

# Migrate to another node (SSE, with local disks)
curl -N -X POST http://192.168.0.11:8080/api/vm/120/set/node \
  -H "Content-Type: application/json" -d '{"node": "pve2"}'
```

### Advanced: Consume the SSE Stream in Go

```go
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

type event struct {
	Step    string `json:"step"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func main() {
	body, err := json.Marshal(map[string]any{
		"os":      "ubuntu",
		"version": "24.04",
		"cpu":     2,
		"ram":     4096,
		"disk":    "32G",
	})
	if err != nil {
		log.Fatal(err)
	}

	resp, err := http.Post("http://192.168.0.11:8080/api/vm/install", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("install rejected: %s", resp.Status)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "event: close" {
			// server-side flow finished
			return
		}
		payload, ok := strings.CutPrefix(line, "data: ")
		if !ok {
			continue
		}
		var e event
		if err := json.Unmarshal([]byte(payload), &e); err != nil {
			continue
		}
		fmt.Printf("[%s] %s %s\n", e.Status, e.Step, e.Message)
		if e.Status == "error" {
			// the failed step explains the reason in message
			log.Printf("step failed: %s", e.Message)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
}
```

## API Reference

Every route is prefixed with `/api`. The server also serves the OS init scripts under `/sh/*` for new VMs to download during initialization.

### Endpoints

| Method | Path | Response | `ALLOW_IPS` | Required VM State | Description |
|--------|------|----------|-------------|-------------------|-------------|
| `GET` | `/health` | text `ok` | — | — | Health check |
| `GET` | `/vm/list` | JSON | — | — | Cluster VM list and node utilization |
| `GET` | `/vm/:id/status` | text | — | — | `qm status` result (`running` / `stopped`) |
| `POST` | `/vm/install` | SSE | ✓ | — | Create and initialize a new VM |
| `POST` | `/vm/:id/start` | SSE | ✓ | stopped | Start and wait for SSH |
| `POST` | `/vm/:id/reboot` | SSE | ✓ | running | Reboot and wait for SSH |
| `POST` | `/vm/:id/shutdown` | text `ok` | ✓ | running | ACPI graceful shutdown |
| `POST` | `/vm/:id/stop` | text `ok` | ✓ | running | Force stop |
| `POST` | `/vm/:id/destroy` | text `ok` | ✓ | stopped | Destroy the VM |
| `POST` | `/vm/:id/set/cpu` | text `ok` | ✓ | stopped | Set vCPU cores |
| `POST` | `/vm/:id/set/memory` | text `ok` | ✓ | stopped | Set memory and apply ballooning per `VM_BALLOON_MIN` |
| `POST` | `/vm/:id/set/disk` | text `ok` | ✓ | stopped | Grow `scsi0` by an increment |
| `POST` | `/vm/:id/set/node` | SSE | ✓ | stopped | Migrate to the target node (`--with-local-disks`) |

VMIDs listed in `.go_qemu_disabled` always get `400` from state-checked endpoints.

### `POST /vm/install` Request Fields

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `os` | string | Yes | — | `debian` / `ubuntu` / `rockylinux` |
| `version` | string | Yes | — | Debian `11` / `12` / `13`; Ubuntu `20.04` / `22.04` / `24.04`; Rocky Linux `8` / `9` / `10` |
| `id` | int | No | auto | Explicit VMID, which also sets the last IP octet |
| `name` | string | No | VMID | Final name becomes `<name>-<vmid>` |
| `node` | string | No | main node | Migrate to this node after creation, before first boot |
| `cpu` | int | No | `1` | Minimum 1, capped by `VM_MAX_CPU` |
| `ram` | int | No | `512` | MB; minimum 512, capped by `VM_MAX_RAM` |
| `disk` | string | No | `16G` | In `G`; minimum 16G, capped by `VM_MAX_DISK` |
| `user` | string | No | per OS | Cloud-init user; defaults to `debian` / `ubuntu` / `rocky` |
| `passwd` | string | No | `passwd` | Cloud-init user password |
| `pubkey` | string | No | — | Extra SSH public key to inject |

The server overwrites `ip`, `gateway`, and `storage` from `GATEWAY`, the VMID, and `ASSIGN_STORAGE`; values sent by clients have no effect.

### Resize Request Fields

| Endpoint | Field | Validation |
|----------|-------|------------|
| `/set/cpu` | `cpu` int | required, 1–32 |
| `/set/memory` | `memory` int | required, 512–32768 (MB) |
| `/set/disk` | `disk` string | increment such as `"16G"` |
| `/set/node` | `node` string | required, target node name |

### `GET /vm/list`

| Query | Value | Description |
|-------|-------|-------------|
| `disable` | omitted | Return everything, disabled entries included |
| | `0` | Running and not disabled only |
| | `1` | Stopped and not disabled only |

```json
{
  "count": 1,
  "list": [
    { "vmid": 120, "name": "web-120", "os": "debian", "running": true, "node": "pve1",
      "cpu": 2, "disk": 32, "memory": 4, "memory_used": 1 }
  ],
  "cluster": [
    { "node": "pve1", "max_cpu": 16, "max_memory": 62.7, "cpu": 12.5, "memory": 6.38,
      "memory_used": 1.59, "disk": 93.93, "running": true }
  ],
  "data": []
}
```

- `list[].disk` / `memory` / `memory_used` are in GiB (integer-truncated)
- `cluster[].cpu` / `memory` / `memory_used` are allocated percentages of node capacity; `max_memory` / `disk` are in GiB
- `data` mirrors `list` and is deprecated

### SSE Event Format

```text
data: {"step":"<stage>","status":"<status>","message":"<message>"}

event: close
data: {}
```

| `status` | Meaning |
|----------|---------|
| `processing` | Step in progress or command output |
| `success` | Step finished; message includes elapsed time |
| `info` | Final notice |
| `error` | Step failed; the flow stops |

`message` prefixes: `[*]` info, `[+]` success, `[-]` failure. The stream ends with `event: close`.

### Install Pipeline Stages

| Stage | Steps |
|-------|-------|
| `preparation` | Assign VMID → derive IP → clamp CPU/RAM → clamp disk → apply defaults → check storage pool |
| `OS preparation` | Resolve image URL → HEAD check → download to `/tmp` (reused when present) |
| `SSH preparation` | Check the main node's SSH public key, generate one when missing |
| `VM creation` | `qm create` (CPU type from cluster baseline) → `qm importdisk` |
| `VM initialization` | Inject SSH keys, password, tag, disk, cloud-init, boot order, disk resize, IP → (optional) migrate → start → wait for SSH → run `/sh/<os>_<version>.sh` → reboot → wait for SSH |

When disk import, VM initialization, or SSH initialization fails, the server force-stops the VM and runs `qm destroy --purge` to remove the partial build.

### OS Init Scripts

New VMs download `sh/<os>_<version>.sh` from `http://<NODE_main>:<PORT>/sh/...` and run it with `sudo bash`. The scripts set the root password and disable `passwd` / `chpasswd`, switch package sources (Debian / Ubuntu) or enable EPEL (Rocky Linux), upgrade the system and install common tools plus `qemu-guest-agent`, set the timezone to `Asia/Taipei` (Debian / Ubuntu also set the `en_US.UTF-8` locale), tune sysctl, and create a 2G swap file. Edit the matching script to customize provisioning.

***

©️ 2025 [邱敬幃 Pardn Chiu](https://www.linkedin.com/in/pardnchiu)
