# QemuRun-pve - 架構

> 返回 [README](./README.zh.md)

## 概覽

```mermaid
graph TB
    Client[客戶端] -->|HTTP / SSE| Gin[Gin 路由 + CORS]
    Gin --> Handler[Handler 層<br/>ALLOW_IPS / 狀態檢查]
    Handler --> Util[Util<br/>VM / 節點查詢]
    Handler --> Service[Service 層]
    Util -->|pvesh| PVE[(Proxmox 叢集 API)]
    Util --> Disabled[.go_qemu_disabled]
    Service -->|qm / pvesh / pvesm| Main[主節點]
    Service -->|SSH root@NODE_x + qm| Remote[遠端節點]
    Service -->|HTTP| Mirror[官方 Cloud Image 鏡像站]
    Service -->|SSH| VM[新 VM]
    VM -->|curl /sh/os_version.sh| Gin
```

## Module: 進入點與設定（cmd/api、internal/config）

載入環境變數、註冊中介層與路由，並以靜態路徑對外提供 OS 初始化腳本。

```mermaid
graph TB
    subgraph Entry[cmd/api]
        Main[main] --> Env[godotenv 載入 .env]
        Env --> Check{PORT / GATEWAY<br/>是否存在}
        Check -->|否| Fatal[log.Fatal]
        Check -->|是| Engine[gin.Default]
        Engine --> Static["/sh 靜態目錄"]
    end
    subgraph Config[internal/config]
        CORS[CORS 中介層] --> Private{來源為私有 IP<br/>且同子網}
        Private -->|是| Allow[回傳該 Origin]
        Private -->|否| Deny[不設定 Allow-Origin]
        Routes[NewRoutes] --> API["/api 群組"]
        API --> VMGroup["/api/vm/:id 群組"]
    end
    Engine --> CORS
    Engine --> Routes
    Main --> NewService[service.NewService]
    NewService --> NewHandler[handler.NewHandler]
    NewHandler --> Routes
```

## Module: Handler（internal/handler）

解析路徑參數與請求內容，執行存取與狀態前置檢查後交給 Service；依端點型態回應純文字、JSON 或 SSE。

```mermaid
graph TB
    subgraph Handler
        Req[請求] --> IPCheck{util.CheckIP<br/>ALLOW_IPS}
        IPCheck -->|拒絕| R403[403 / SSE 錯誤訊息]
        IPCheck -->|通過| Parse[解析 :id]
        Parse --> IDCheck{util.CheckID<br/>停用清單 / 執行狀態}
        IDCheck -->|不符| R400[400 / SSE 錯誤訊息]
        IDCheck -->|符合| Bind[ShouldBindJSON 驗證]
        Bind --> Dispatch[呼叫 Service]
    end
    subgraph Sync[同步端點 qemu.go]
        Stop
        Shutdown
        Destroy
        CPU
        Memory
        Disk
    end
    subgraph Stream[SSE 端點 qemuSSE.go]
        Install
        Start
        Reboot
        Node
    end
    subgraph Query[查詢端點 main.go]
        GetStatus
        GetVMList
    end
    Dispatch --> Sync
    Dispatch --> Stream
    GetVMList --> Agg[合併 VM 與節點<br/>計算配置百分比]
```

## Module: Service（internal/service）

封裝所有 `qm`／`pvesh`／`pvesm` 呼叫，負責安裝流程、資源配置與多節點指令轉送。

```mermaid
graph TB
    subgraph Service
        Install[Install 流程] --> AssignIP[assignIP<br/>雙端並行探測]
        Install --> OSImage[getOSImage<br/>check / download]
        Install --> Pubkey[checkUserPubkey<br/>createUserSSHKeyPair]
        Install --> CreateVM[createVM]
        CreateVM --> CPUType[GetClusterCPUType]
        CPUType --> Cache[.go_qemu_cpu_type]
        Install --> InitVM[initialVM<br/>Cloud-Init 設定]
        InitVM --> AdminKey[.go_qemu_pubkey_admin]
        Install --> InitSSH[initialWithSSH]
        Install --> Alive[CheckAlive<br/>SSH 輪詢 60 次]
        Install --> Clean[clean<br/>失敗時清除]

        Lifecycle[Start / Reboot / Stop<br/>Shutdown / Destroy] --> Locate[getVMIDsNode]
        Resize[CPU / Memory / Disk] --> Locate
        Migrate[Node 遷移] --> Locate
        Locate --> GetCmd[getCommand]
        GetCmd -->|主節點| QM[qm ...]
        GetCmd -->|其他節點| SSHQM[ssh root@NODE_x qm ...]
        Install --> SSE[SSE 推送]
        Lifecycle --> SSE
        Migrate --> RunSSE[runCommandSSE<br/>stdout / stderr 轉串流]
        RunSSE --> SSE
    end
    Locate --> Conf["/etc/pve/nodes/*/qemu-server/*.conf"]
```

### VMID／IP 配置

```mermaid
graph LR
    subgraph assignIP
        Range[ASSIGN_IP_START..END<br/>夾在 100..254] --> Pair[每輪同時派出<br/>start++ 與 end--]
        Pair --> Sem[信號量 10 並行]
        Sem --> C1{已存在 .conf?}
        C1 -->|是| Skip[略過]
        C1 -->|否| C2{qm config 成功?}
        C2 -->|是| Skip
        C2 -->|否| C3{IP:22 可連線?}
        C3 -->|是| Skip
        C3 -->|否| Found[送出結果並 cancel 其他 goroutine]
    end
    Found --> Result[VMID = IP 末段]
    Range -.->|10 秒逾時| Timeout[回傳錯誤]
```

### 叢集 CPU 基準

```mermaid
graph LR
    subgraph GetClusterCPUType
        Cached{快取檔存在?} -->|是| Return[直接回傳]
        Cached -->|否| Nodes[pvesh get /nodes]
        Nodes --> Each[逐節點 pvesh get /nodes/x/status]
        Each --> Flags[解析 cpuinfo.flags]
        Flags --> Level[判定 x86-64-v1 ~ v4]
        Level --> Min[取全叢集最低等級]
        Min --> Write[寫入 .go_qemu_cpu_type]
    end
    Write --> Create[qm create --cpu]
    Nodes -.->|失敗| Fallback[kvm64]
```

## Module: Util 與 Model（internal/util、internal/model）

Util 提供跨 Handler 共用的叢集查詢與存取檢查；Model 定義請求、回應與 SSE 結構。

```mermaid
classDiagram
    class Config {
        +int ID
        +string Name
        +string Node
        +string Storage
        +string OS
        +string Version
        +int CPU
        +string Disk
        +int RAM
        +string IP
        +string Gateway
        +string User
        +string Passwd
        +string Pubkey
    }
    class VM {
        +int ID
        +string Name
        +string OS
        +bool Running
        +string Node
        +int CPU
        +int Disk
        +int Memory
        +int MemoryUsed
    }
    class Node {
        +string Node
        +float64 MaxCPU
        +float64 MaxMemory
        +float64 CPU
        +float64 Memory
        +float64 MemoryUsed
        +float64 Disk
        +bool Running
    }
    class SSE {
        +string Step
        +string Status
        +string Message
    }
    class Status {
        +string IP
        +bool Available
        +int VMID
    }
    class util {
        +CheckIP(ip) bool
        +CheckID(vmid, running) (int, map, error)
        +GetVMMap() map
        +GetNodeMap() map
        +GetOSUser(vmid) string
        +IncludeVM(isRunning, os, disable) bool
    }
    util ..> VM
    util ..> Node
```

## 資料流

### 建立 VM

```mermaid
sequenceDiagram
    participant C as 客戶端
    participant H as Handler
    participant S as Service
    participant PVE as 主節點 qm/pvesh
    participant R as 目標節點
    participant VM as 新 VM

    C->>H: POST /api/vm/install
    H->>H: CheckIP + 綁定 Config
    H->>S: Install(config)
    S-->>C: SSE 開始
    S->>S: assignIP（未指定 id）
    S->>S: 套用 CPU / RAM / 磁碟上下限與預設值
    S->>PVE: pvesm status（檢查儲存池）
    S->>S: 下載 Cloud Image 至 /tmp
    S->>PVE: qm create --cpu <叢集基準>
    S->>PVE: qm importdisk
    S->>PVE: qm set（金鑰 / 密碼 / Tag / 磁碟 / Cloud-Init / IP）
    S->>PVE: qm resize（最多重試 3 次）
    opt 指定 node
        S->>PVE: qm migrate --with-local-disks
        PVE->>R: 遷移
    end
    S->>R: qm start（本機或經 SSH）
    loop 最多 60 次，每 5 秒
        S->>VM: ssh echo ready
    end
    VM->>H: curl /sh/<os>_<version>.sh
    S->>VM: ssh 執行初始化腳本
    S-->>C: SSE 轉送腳本輸出
    S->>R: qm reboot
    S->>VM: 等待 SSH 就緒
    S-->>C: SSE VMID / IP / User
    H-->>C: event: close
```

### 同步操作（以 Stop 為例）

```mermaid
sequenceDiagram
    participant C as 客戶端
    participant H as Handler
    participant U as Util
    participant S as Service
    participant N as VM 所在節點

    C->>H: POST /api/vm/120/stop
    H->>H: CheckIP
    H->>U: CheckID(120, false)
    U->>U: pvesh get /cluster/resources + .go_qemu_disabled
    U-->>H: 通過
    H->>S: Stop(120)
    S->>S: getVMIDsNode 讀取 /etc/pve/nodes
    alt 主節點
        S->>N: qm stop 120
    else 遠端節點
        S->>N: ssh root@NODE_x qm stop 120
    end
    S-->>H: nil
    H-->>C: 200 ok
```

## 狀態機

### VM 生命週期（經 API 可觸發的轉換）

```mermaid
stateDiagram-v2
    [*] --> Installing: POST /vm/install
    Installing --> Running: SSH 就緒
    Installing --> [*]: 失敗並 purge
    Running --> Stopped: shutdown / stop
    Running --> Running: reboot
    Stopped --> Running: start
    Stopped --> Stopped: set/cpu、set/memory、set/disk、set/node
    Stopped --> [*]: destroy
    Running --> Disabled: 寫入 .go_qemu_disabled
    Stopped --> Disabled: 寫入 .go_qemu_disabled
    note right of Disabled: 自清單移除後回到實際執行狀態
```

***

©️ 2025 [邱敬幃 Pardn Chiu](https://www.linkedin.com/in/pardnchiu)
