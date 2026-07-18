# smonitor — S.M.A.R.T 硬盘健康检查工具

Windows 10/11 兼容、单 .exe 分发。读取物理硬盘的 S.M.A.R.T 属性与 NVMe 健康日志，按权威阈值判标；违规时弹出红色告警窗；一键复制报表。

## 运行

1. 拿到 `smonitor.exe`（双击）
2. 由于要直接访问 `\\.\PhysicalDriveN`，会弹出 UAC 请求管理员权限 → **允许**
3. 主窗口显示所有 SMART 属性与状态；红色告警条列出违规项；点击「一键复制报表」贴到任意地方

## 构建

```bat
.\build.bat
```

产物：`smonitor.exe`（单文件，无额外 DLL）

## 采集方式

- **主路径 — IOCTL**：枚举 `PhysicalDrive0..255`，通过 `IOCTL_STORAGE_QUERY_PROPERTY` 读取型号/序列号/总线类型；ATA 使用 `IOCTL_SMART_RCV_DRIVE_DATA` 与 ATA pass-through，NVMe 使用 `IOCTL_STORAGE_PROTOCOL_COMMAND` 读取 Health Log Page 0x02。
- **兼容回退 — WMI**：对未读到属性或 ATA SMART 数据校验和异常的磁盘，通过 `root\WMI\MSStorageDriver_FailurePredictData` / `FailurePredictThresholds` 读取属性和阈值；已成功采集的数据不会被覆盖。
- **NVMe SSD**：`IOCTL_STORAGE_PROTOCOL_COMMAND`（Get Log Page 0x02，健康日志）。
- 不需要 smartctl.exe / 不需要任何外部依赖。

## 告警规则

### ATA
| 属性 | 说明 | 条件 | 严重度 |
|---|---|---|---|
| 0x05 Reallocated_Sector_Ct | 重映射扇区计数 | raw != 0 | 严重 |
| 0xC5 Current_Pending_Sector | 等待重映射扇区 | raw != 0 | 警告 |
| 0xC6 Offline_Uncorrectable | 不可纠正错误 | raw != 0 | 警告 |
| 0xBB Reported_Uncorrectable_Errors | 报告的不可纠正错误 | raw != 0 | 严重 |
| 0x01 Raw_Read_Error_Rate | 读取错误率 | `value <= device threshold` | 警告（避免 WD/Seagate raw 误报）|
| 其他未单列属性 | 厂商/设备特定 | `value <= device threshold` | Pre-fail=严重，Old-age=警告 |
| 0xC2 Temperature | 温度 | > 60°C | 严重 |
| 0xC2 Temperature | 温度 | > 55°C | 警告 |
| 0xBC Command_Timeout | 命令超时 | raw > 10 | 警告 |
| 0xE9 Media_Wearout_Indicator | SSD 寿命 | <= 10% 严重；<= 20% 警告 |
| 0xE8 Available_Reservd_Space | 预留块/寿命 | <= 10% | 警告 |

### NVMe
| 字段 | 条件 | 严重度 |
|---|---|---|
| Media_Data_Integrity_Errors ("0E") | != 0 | 严重 |
| Percentage_Used | >= 100% 严重；>= 80% 警告 |
| Temperature | > 60°C | 严重 |
| Temperature Sensor 1–8 | > 60°C | 严重 |
| Critical_Warning | != 0 | 严重 |
| Endurance_Group_Critical_Warning_Summary | != 0 | 严重 |
| Available_Spare < Threshold | | 严重 |
| Read_Only_Mode | != 0 | 严重 |

ATA SMART 数据页校验和异常会作为数据完整性警告显示，并触发 WMI 回退；不会被当作“健康”。

## 严谨性说明

- **0x01 Read Error Rate 在 WD/Seagate 健康盘上 raw 也几乎必非零**（WD 是打包复合值，Seagate 低 32 位计可纠正 ECC），因此只按设备报告的归一化阈值判定。
- **0x0E / 0xBC 在 ATA 规范中未统一定义**；0x0E 不设默认告警，0xBC 的累计值超过 10 时作为兼容性警告显示。
- **0x05 与 0xBB** 是 ATA 失效的最可靠指标，任何非零均视为风险。
- **NVMe 路径**中的 "Media and Data Integrity Errors" 字段才是用户定义的 0E 语义在 NVMe 中的真正对应。

## 项目结构

```
.
├── main.go                # 入口
├── smart/                 # SMART 采集（纯 Go Win32 IOCTL）
│   ├── types.go           # 统一数据结构
│   ├── win32.go           # IOCTL 常量与设备打开
│   ├── ata.go             # ATA SMART / IDENTIFY
│   ├── nvme.go            # NVMe Get Log Page
│   ├── enum.go            # 磁盘枚举
│   └── attrnames.go       # ATA 属性 ID<->名 权威映射
├── health/
│   └── rules.go           # 阈值判断
├── ui/                    # GUI（纯 Go，lxn/walk）
│   ├── report.go          # 主报表窗
│   ├── alert.go           # 红色告警窗
│   ├── clipboard.go       # 剪贴板（Win32 syscall）
│   └── textreport.go      # 文本报表生成
├── rsrc/app.manifest      #管理员权限/视觉样式/DPI
├── build.bat
└── README.md
```

## 验证方法

1. 与 smartmontools `smartctl -a /dev/sdX` 对照属性值一致
2. 在已知坏盘上 0x05!=0 触发红色告警
3. NVMe 盘能显示 Temperature / PercentageUsed / MediaErrors
4. 复制报表粘贴到记事本/微信无乱码（UTF-16 BOM）
5. 对支持两条路径的 ATA 盘，交叉比对 IOCTL 与 WMI 的属性 ID、Raw、当前值、最差值和阈值；两者应一致。

### 无需坏盘的异常模拟

在 GUI 中点击“模拟异常验证”即可直接查看红黄表格、告警窗口和异常复制格式。该操作只加载内存中的固定数据，不读取或写入真实硬盘；点击“重新扫描”会恢复真实磁盘数据。

下面的测试构造 ATA 重映射扇区、过热、命令超时、厂商 Pre-fail，以及 NVMe 临界位图、介质错误、传感器过热和寿命告警；同时验证“复制异常条目”的 TSV 内容不包含健康属性：

```powershell
go test -run TestSimulatedDiskFailuresProduceFeedbackReport -v .
```
