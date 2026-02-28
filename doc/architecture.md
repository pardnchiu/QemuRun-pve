# QemuRun-pve - Architecture

> Back to [README](../README.md)

## Overview

```mermaid
graph TB
    Client -->|HTTP / SSE| Gin[Gin Router + CORS]
    Gin --> Handler[Handler Layer<br/>ALLOW_IPS / state checks]
    Handler --> Util[Util<br/>VM / node lookup]
    Handler --> Service[Service Layer]
    Util -->|pvesh| PVE[(Proxmox Cluster API)]
    Util --> Disabled[.go_qemu_disabled]
    Service -->|qm / pvesh / pvesm| Main[Main Node]
    Service -->|SSH root@NODE_x + qm| Remote[Remote Node]
    Service -->|HTTP| Mirror[Official Cloud Image Mirrors]
    Service -->|SSH| VM[New VM]
    VM -->|curl /sh/os_version.sh| Gin
```

## Module: Entry and Config (cmd/api, internal/config)

Loads environment variables, registers middleware and routes, and serves the OS init scripts as static files.

```mermaid
graph TB
    subgraph Entry[cmd/api]
        Main[main] --> Env[godotenv loads .env]
        Env --> Check{PORT / GATEWAY<br/>present?}
        Check -->|no| Fatal[log.Fatal]
        Check -->|yes| Engine[gin.Default]
        Engine --> Static["/sh static dir"]
    end
    subgraph Config[internal/config]
        CORS[CORS middleware] --> Private{origin is private IP<br/>in same subnet?}
        Private -->|yes| Allow[echo Origin]
        Private -->|no| Deny[no Allow-Origin]
        Routes[NewRoutes] --> API["/api group"]
        API --> VMGroup["/api/vm/:id group"]
    end
    Engine --> CORS
    Engine --> Routes
    Main --> NewService[service.NewService]
    NewService --> NewHandler[handler.NewHandler]
    NewHandler --> Routes
```

## Module: Handler (internal/handler)

Parses path params and bodies, runs access and state pre-checks, then delegates to Service; responds with plain text, JSON, or SSE depending on the endpoint.

```mermaid
graph TB
    subgraph Handler
        Req[Request] --> IPCheck{util.CheckIP<br/>ALLOW_IPS}
        IPCheck -->|denied| R403[403 / SSE error]
        IPCheck -->|passed| Parse[parse :id]
        Parse --> IDCheck{util.CheckID<br/>disabled list / run state}
        IDCheck -->|mismatch| R400[400 / SSE error]
        IDCheck -->|match| Bind[ShouldBindJSON validation]
        Bind --> Dispatch[call Service]
    end
    subgraph Sync[sync endpoints qemu.go]
        Stop
        Shutdown
        Destroy
        CPU
        Memory
        Disk
    end
    subgraph Stream[SSE endpoints qemuSSE.go]
        Install
        Start
        Reboot
        Node
    end
    subgraph Query[query endpoints main.go]
        GetStatus
        GetVMList
    end
    Dispatch --> Sync
    Dispatch --> Stream
    GetVMList --> Agg[merge VMs and nodes<br/>compute allocation %]
```

## Module: Service (internal/service)

Wraps every `qm` / `pvesh` / `pvesm` call and owns the install pipeline, resource allocation, and multi-node command dispatch.

```mermaid
graph TB
    subgraph Service
        Install[Install pipeline] --> AssignIP[assignIP<br/>two-ended concurrent probe]
        Install --> OSImage[getOSImage<br/>check / download]
        Install --> Pubkey[checkUserPubkey<br/>createUserSSHKeyPair]
        Install --> CreateVM[createVM]
        CreateVM --> CPUType[GetClusterCPUType]
        CPUType --> Cache[.go_qemu_cpu_type]
        Install --> InitVM[initialVM<br/>cloud-init setup]
        InitVM --> AdminKey[.go_qemu_pubkey_admin]
        Install --> InitSSH[initialWithSSH]
        Install --> Alive[CheckAlive<br/>SSH poll x60]
        Install --> Clean[clean<br/>purge on failure]

        Lifecycle[Start / Reboot / Stop<br/>Shutdown / Destroy] --> Locate[getVMIDsNode]
        Resize[CPU / Memory / Disk] --> Locate
        Migrate[Node migration] --> Locate
        Locate --> GetCmd[getCommand]
        GetCmd -->|main node| QM[qm ...]
        GetCmd -->|other node| SSHQM[ssh root@NODE_x qm ...]
        Install --> SSE[SSE push]
        Lifecycle --> SSE
        Migrate --> RunSSE[runCommandSSE<br/>stdout / stderr to stream]
        RunSSE --> SSE
    end
    Locate --> Conf["/etc/pve/nodes/*/qemu-server/*.conf"]
```

### VMID / IP Allocation

```mermaid
graph LR
    subgraph assignIP
        Range[ASSIGN_IP_START..END<br/>clamped to 100..254] --> Pair[each round dispatches<br/>start++ and end--]
        Pair --> Sem[semaphore of 10]
        Sem --> C1{.conf exists?}
        C1 -->|yes| Skip[skip]
        C1 -->|no| C2{qm config succeeds?}
        C2 -->|yes| Skip
        C2 -->|no| C3{IP:22 reachable?}
        C3 -->|yes| Skip
        C3 -->|no| Found[send result and cancel other goroutines]
    end
    Found --> Result[VMID = last IP octet]
    Range -.->|10s timeout| Timeout[return error]
```

### Cluster CPU Baseline

```mermaid
graph LR
    subgraph GetClusterCPUType
        Cached{cache file exists?} -->|yes| Return[return cached]
        Cached -->|no| Nodes[pvesh get /nodes]
        Nodes --> Each[pvesh get /nodes/x/status per node]
        Each --> Flags[parse cpuinfo.flags]
        Flags --> Level[classify x86-64-v1 ~ v4]
        Level --> Min[take cluster-wide minimum]
        Min --> Write[write .go_qemu_cpu_type]
    end
    Write --> Create[qm create --cpu]
    Nodes -.->|failure| Fallback[kvm64]
```

## Module: Util and Model (internal/util, internal/model)

Util provides the cluster lookups and access checks shared by handlers; Model defines request, response, and SSE structures.

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

## Data Flow

### Create a VM

```mermaid
sequenceDiagram
    participant C as Client
    participant H as Handler
    participant S as Service
    participant PVE as Main Node qm/pvesh
    participant R as Target Node
    participant VM as New VM

    C->>H: POST /api/vm/install
    H->>H: CheckIP + bind Config
    H->>S: Install(config)
    S-->>C: SSE start
    S->>S: assignIP (when id omitted)
    S->>S: clamp CPU / RAM / disk and apply defaults
    S->>PVE: pvesm status (check storage pool)
    S->>S: download cloud image to /tmp
    S->>PVE: qm create --cpu <cluster baseline>
    S->>PVE: qm importdisk
    S->>PVE: qm set (keys / password / tag / disk / cloud-init / IP)
    S->>PVE: qm resize (up to 3 retries)
    opt node specified
        S->>PVE: qm migrate --with-local-disks
        PVE->>R: migrate
    end
    S->>R: qm start (local or via SSH)
    loop up to 60 times, every 5s
        S->>VM: ssh echo ready
    end
    VM->>H: curl /sh/<os>_<version>.sh
    S->>VM: ssh run init script
    S-->>C: SSE relays script output
    S->>R: qm reboot
    S->>VM: wait for SSH
    S-->>C: SSE VMID / IP / User
    H-->>C: event: close
```

### Sync Operation (Stop)

```mermaid
sequenceDiagram
    participant C as Client
    participant H as Handler
    participant U as Util
    participant S as Service
    participant N as Hosting Node

    C->>H: POST /api/vm/120/stop
    H->>H: CheckIP
    H->>U: CheckID(120, false)
    U->>U: pvesh get /cluster/resources + .go_qemu_disabled
    U-->>H: passed
    H->>S: Stop(120)
    S->>S: getVMIDsNode reads /etc/pve/nodes
    alt main node
        S->>N: qm stop 120
    else remote node
        S->>N: ssh root@NODE_x qm stop 120
    end
    S-->>H: nil
    H-->>C: 200 ok
```

## State Machine

### VM Lifecycle (API-triggered transitions)

```mermaid
stateDiagram-v2
    [*] --> Installing: POST /vm/install
    Installing --> Running: SSH ready
    Installing --> [*]: failure, purged
    Running --> Stopped: shutdown / stop
    Running --> Running: reboot
    Stopped --> Running: start
    Stopped --> Stopped: set/cpu, set/memory, set/disk, set/node
    Stopped --> [*]: destroy
    Running --> Disabled: added to .go_qemu_disabled
    Stopped --> Disabled: added to .go_qemu_disabled
    note right of Disabled: removal from the list restores the actual run state
```

***

©️ 2025 [邱敬幃 Pardn Chiu](https://www.linkedin.com/in/pardnchiu)
