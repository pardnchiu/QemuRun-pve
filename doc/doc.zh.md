# QemuRun-pve - 技術文件
最後更新：2026-10-06

> 返回 [README](./README.zh.md)

## 前置需求

- Proxmox VE 叢集，服務以 `root` 身分執行於**主節點**（需要 `qm`、`pvesh`、`pvesm` 與 `/etc/pve/nodes` 讀取權限）
- Go 1.25 以上（僅自原始碼建置時需要）
- 主節點可透過金鑰以 `root` 免密碼 SSH 至所有其他節點
- 主節點執行身分的 `~/.ssh/` 下存在 `id_ed25519.pub`／`id_rsa.pub`／`id_ecdsa.pub` 其一（缺少時安裝流程會自動以 `ssh-keygen` 建立 ed25519 金鑰）
- 網段規劃為 `/24`，VM 的 IP 末段等於 VMID（例如 VMID `120` → `192.168.0.120`），因此 VMID 範圍需落在 `100`–`254`
- 一個啟用中、類型為 `dir`／`zfspool`／`lvmthin`／`nfs` 的儲存池，且支援 `cloudinit` 磁碟
- 主節點可連外下載官方 Cloud Image（Debian、Rocky Linux、Ubuntu 鏡像站）

## 安裝

### 自原始碼建置（建議）

服務以工作目錄為基準讀取 `.env`、`sh/` 初始化腳本與 `.go_qemu_*` 狀態檔，因此建議直接在 clone 下來的目錄中執行。

```bash
git clone https://github.com/pardnchiu/QemuRun-pve.git
cd QemuRun-pve
cp .env.example .env
touch .go_qemu_disabled
go build -o QemuRun-pve ./cmd/api
./QemuRun-pve
```

啟動後會列出所有已註冊路由，並輸出 `goQemu run at localhost:<PORT>`。

### 使用 Release 執行檔

推送 `v*` tag 時 GitHub Actions 會建置 `linux/amd64` 執行檔並打包為 `QemuRun-pve@<tag>.tar.gz`。解壓出的 `app` 必須與 `sh/` 目錄、`.env`、`.go_qemu_disabled` 放在同一工作目錄。

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

### 以 systemd 常駐

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

## 設定

### 環境變數

啟動時以 `godotenv` 載入工作目錄下的 `.env`；檔案不存在時僅警告，改讀系統環境變數。

| 變數 | 必要 | 預設 | 說明 |
|------|------|------|------|
| `PORT` | 是 | — | API 監聽埠；未設定則啟動失敗 |
| `GATEWAY` | 是 | — | VM 網關，例如 `192.168.0.1`；前三段同時作為 VM IP 前綴 |
| `MAIN_NODE` | 是 | — | 服務所在的 Proxmox 節點名稱 |
| `NODE_<name>` | 是 | — | 每個節點（含主節點）的管理 IP，例如 `NODE_pve2=192.168.0.12` |
| `ALLOW_IPS` | 是 | — | 允許執行寫入操作的來源 IP，逗號分隔；`0.0.0.0` 表示全部放行；留空則全部拒絕 |
| `ASSIGN_STORAGE` | 是 | — | 匯入磁碟與 Cloud-Init 使用的儲存池，例如 `local-zfs` |
| `ASSIGN_IP_START` | 否 | `100` | 自動配置 VMID 的下限，低於 `100` 時以 `100` 計 |
| `ASSIGN_IP_END` | 否 | `254` | 自動配置 VMID 的上限，高於 `254` 時以 `254` 計 |
| `VM_MAX_CPU` | 否 | 不限 | 安裝時 vCPU 上限，超過則自動壓回 |
| `VM_MAX_RAM` | 否 | 不限 | 安裝時記憶體上限（MB） |
| `VM_MAX_DISK` | 否 | 不限 | 安裝時磁碟上限（GB） |
| `VM_BALLOON_MIN` | 否 | 停用 | Balloon 保留記憶體（MB）；設定後記憶體 ≥ 此值 + 1024 時啟用 NUMA 與 Balloon |
| `VM_ROOT_PASSWORD` | 否 | 腳本預設 | 傳給 OS 初始化腳本的 root 密碼；留空時使用腳本內建預設值 `0123456789` |

### 工作目錄狀態檔

| 檔案 | 必要 | 說明 |
|------|------|------|
| `.go_qemu_disabled` | 是 | 停用清單，每行 `<vmid>:<name>`；列入者不可經 API 操作。**檔案不存在時 `/api/vm/list` 與所有需檢查 VM 狀態的端點都會失敗**，無停用項目時建立空檔即可 |
| `.go_qemu_pubkey_admin` | 否 | 額外注入到每台新 VM 的管理者 SSH 公鑰，可多行 |
| `.go_qemu_cpu_type` | 自動 | 叢集 CPU 類型快取；節點硬體變動後刪除即可重新偵測 |

### `.env` 範例

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

## 使用方式

### 基礎

```bash
# 健康檢查
curl http://192.168.0.11:8080/api/health
# ok

# 列出所有 VM 與節點資源使用率
curl http://192.168.0.11:8080/api/vm/list

# 查詢單台 VM 狀態（僅限主節點上的 VM）
curl http://192.168.0.11:8080/api/vm/120/status
# running
```

### 建立 VM

`-N` 關閉 curl 緩衝以即時看到 SSE 進度。未帶 `id` 時自動配置 VMID 與 IP。

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

### 生命週期操作

```bash
# 啟動（SSE，等待 SSH 可連線才結束）
curl -N -X POST http://192.168.0.11:8080/api/vm/120/start

# 重新開機（SSE）
curl -N -X POST http://192.168.0.11:8080/api/vm/120/reboot

# 正常關機／強制停止
curl -X POST http://192.168.0.11:8080/api/vm/120/shutdown
curl -X POST http://192.168.0.11:8080/api/vm/120/stop

# 刪除（VM 需為停止狀態）
curl -X POST http://192.168.0.11:8080/api/vm/120/destroy
```

### 調整資源

以下操作皆要求 VM 處於停止狀態。

```bash
# 設定 vCPU（1–32）
curl -X POST http://192.168.0.11:8080/api/vm/120/set/cpu \
  -H "Content-Type: application/json" -d '{"cpu": 4}'

# 設定記憶體 MB（512–32768）
curl -X POST http://192.168.0.11:8080/api/vm/120/set/memory \
  -H "Content-Type: application/json" -d '{"memory": 8192}'

# 擴充磁碟（增量）
curl -X POST http://192.168.0.11:8080/api/vm/120/set/disk \
  -H "Content-Type: application/json" -d '{"disk": "16G"}'

# 遷移到其他節點（SSE，含本地磁碟）
curl -N -X POST http://192.168.0.11:8080/api/vm/120/set/node \
  -H "Content-Type: application/json" -d '{"node": "pve2"}'
```

### 進階：以 Go 消費 SSE 串流

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
			// 伺服器端流程結束
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
			// 失敗的步驟會在 message 中說明原因
			log.Printf("step failed: %s", e.Message)
		}
	}
	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
}
```

## API 參考

所有路由前綴為 `/api`。服務另以 `/sh/*` 提供 OS 初始化腳本，供新 VM 在初始化階段下載。

### 端點

| 方法 | 路徑 | 回應 | `ALLOW_IPS` | VM 狀態要求 | 說明 |
|------|------|------|-------------|-------------|------|
| `GET` | `/health` | 文字 `ok` | — | — | 健康檢查 |
| `GET` | `/vm/list` | JSON | — | — | 叢集 VM 清單與節點使用率 |
| `GET` | `/vm/:id/status` | 文字 | — | — | `qm status` 結果（`running`／`stopped`） |
| `POST` | `/vm/install` | SSE | ✓ | — | 建立並初始化新 VM |
| `POST` | `/vm/:id/start` | SSE | ✓ | 停止中 | 啟動並等待 SSH 就緒 |
| `POST` | `/vm/:id/reboot` | SSE | ✓ | 執行中 | 重開並等待 SSH 就緒 |
| `POST` | `/vm/:id/shutdown` | 文字 `ok` | ✓ | 執行中 | ACPI 正常關機 |
| `POST` | `/vm/:id/stop` | 文字 `ok` | ✓ | 執行中 | 強制停止 |
| `POST` | `/vm/:id/destroy` | 文字 `ok` | ✓ | 停止中 | 刪除 VM |
| `POST` | `/vm/:id/set/cpu` | 文字 `ok` | ✓ | 停止中 | 設定 vCPU 核心數 |
| `POST` | `/vm/:id/set/memory` | 文字 `ok` | ✓ | 停止中 | 設定記憶體，並依 `VM_BALLOON_MIN` 設定 Balloon |
| `POST` | `/vm/:id/set/disk` | 文字 `ok` | ✓ | 停止中 | 以增量擴充 `scsi0` |
| `POST` | `/vm/:id/set/node` | SSE | ✓ | 停止中 | 遷移至指定節點（`--with-local-disks`） |

列於 `.go_qemu_disabled` 的 VMID 在需檢查狀態的端點一律回 `400`。

### `POST /vm/install` 請求欄位

| 欄位 | 型別 | 必要 | 預設 | 說明 |
|------|------|------|------|------|
| `os` | string | 是 | — | `debian`／`ubuntu`／`rockylinux` |
| `version` | string | 是 | — | Debian `11`／`12`／`13`；Ubuntu `20.04`／`22.04`／`24.04`；Rocky Linux `8`／`9`／`10` |
| `id` | int | 否 | 自動配置 | 指定 VMID，同時決定 IP 末段 |
| `name` | string | 否 | VMID | 實際名稱為 `<name>-<vmid>` |
| `node` | string | 否 | 主節點 | 建立完成後遷移到此節點再開機 |
| `cpu` | int | 否 | `1` | 下限 1，上限 `VM_MAX_CPU` |
| `ram` | int | 否 | `512` | MB；下限 512，上限 `VM_MAX_RAM` |
| `disk` | string | 否 | `16G` | 以 `G` 為單位；下限 16G，上限 `VM_MAX_DISK` |
| `user` | string | 否 | 依 OS | Cloud-Init 使用者；預設 `debian`／`ubuntu`／`rocky` |
| `passwd` | string | 否 | `passwd` | Cloud-Init 使用者密碼 |
| `pubkey` | string | 否 | — | 額外注入的 SSH 公鑰 |

`ip`、`gateway`、`storage` 會由伺服器依 `GATEWAY`、VMID、`ASSIGN_STORAGE` 覆寫，傳入無效。

### 資源調整請求欄位

| 端點 | 欄位 | 驗證 |
|------|------|------|
| `/set/cpu` | `cpu` int | 必填，1–32 |
| `/set/memory` | `memory` int | 必填，512–32768（MB） |
| `/set/disk` | `disk` string | 增量大小，例如 `"16G"` |
| `/set/node` | `node` string | 必填，目標節點名稱 |

### `GET /vm/list`

| Query | 值 | 說明 |
|-------|----|------|
| `disable` | 省略 | 回傳全部（含停用清單） |
| | `0` | 僅執行中且未停用 |
| | `1` | 僅已停止且未停用 |

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

- `list[].disk`／`memory`／`memory_used` 單位為 GiB（整數截斷）
- `cluster[].cpu`／`memory`／`memory_used` 為已配置量佔節點總量的百分比；`max_memory`／`disk` 單位為 GiB
- `data` 與 `list` 內容相同，為已棄用欄位

### SSE 事件格式

```text
data: {"step":"<階段>","status":"<狀態>","message":"<訊息>"}

event: close
data: {}
```

| `status` | 意義 |
|----------|------|
| `processing` | 步驟進行中或指令輸出 |
| `success` | 步驟完成，訊息附耗時 |
| `info` | 結束提示 |
| `error` | 步驟失敗，流程中止 |

`message` 前綴：`[*]` 資訊、`[+]` 成功、`[-]` 失敗。串流以 `event: close` 結束。

### 安裝流程階段

| 階段 | 步驟 |
|------|------|
| `preparation` | 配置 VMID → 計算 IP → 套用 CPU／RAM 上下限 → 套用磁碟上下限 → 預設值 → 檢查儲存池 |
| `OS preparation` | 解析映像 URL → HEAD 檢查 → 下載至 `/tmp`（已存在則重用） |
| `SSH preparation` | 檢查主節點 SSH 公鑰，缺少則產生 |
| `VM creation` | `qm create`（CPU 類型取叢集基準）→ `qm importdisk` |
| `VM initialization` | 注入 SSH 金鑰、密碼、Tag、磁碟、Cloud-Init、開機順序、磁碟擴充、IP → （選）遷移 → 開機 → 等待 SSH → 執行 `/sh/<os>_<version>.sh` → 重開 → 等待 SSH |

匯入磁碟、初始化設定或 SSH 初始化失敗時，會強制停止並 `qm destroy --purge` 清除半成品 VM。

### OS 初始化腳本

`sh/<os>_<version>.sh` 由新 VM 透過 `http://<NODE_主節點>:<PORT>/sh/...` 下載並以 `sudo bash` 執行，內容包含：設定 root 密碼並停用 `passwd`／`chpasswd`、切換套件來源（Debian／Ubuntu）或啟用 EPEL（Rocky Linux）、升級系統並安裝常用工具與 `qemu-guest-agent`、設定時區 `Asia/Taipei`（Debian／Ubuntu 另設語系 `en_US.UTF-8`）、調整 sysctl、建立 2G swap。依需求修改對應腳本即可客製化。

***

©️ 2025 [邱敬幃 Pardn Chiu](https://www.linkedin.com/in/pardnchiu)
