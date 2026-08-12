# smonitor — S.M.A.R.T 硬盘健康检查工具

Windows 10/11 兼容、单 .exe 分发。读取物理硬盘的 S.M.A.R.T 属性与 NVMe 健康日志，按权威阈值判标；异常始终在主窗口顶部横幅和表格中显示，不弹出阻塞式报警窗；可一键复制异常条目。

## 运行

1. 拿到 `smonitor.exe`（双击）
2. 由于要直接访问 `\\.\PhysicalDriveN`，会弹出 UAC 请求管理员权限 → **允许**
3. 主窗口立即显示并开始后台扫描（USB/RAID 探测可能耗时数秒，窗口不会卡住）；完成后列出所有 SMART 属性与状态，顶部横幅列出异常项；点击「复制异常条目」可直接粘贴到反馈表格。点击「模拟异常验证」可在不操作真实磁盘的前提下检查异常显示、横幅和复制格式。

## 构建

```bat
.\build.bat
```

产物：`smonitor.exe`（单文件，无额外 DLL）

### 代码签名（可选，自动跳过）

`build.bat` 在构建后自动签名，按以下优先级查找证书；都没有时静默跳过，不影响构建：

1. **PFX 文件**：设置环境变量后直接构建：

   ```bat
   set SMONITOR_PFX=C:\path\cert.pfx
   set SMONITOR_PFX_PASSWORD=密码      （可省略，省略则交互输入）
   .\build.bat
   ```

2. **证书存储**：`CurrentUser\My` / `LocalMachine\My` 中第一个有效期最长的 Code Signing 证书（自动选用，无需配置）。

签名使用 SHA256 + RFC 3161 时间戳（`/fd SHA256 /tr http://timestamp.digicert.com /td SHA256`）。需要 Windows SDK 的 `signtool.exe`（Windows 10 SDK 自带）。

> 注：签名只消除 SmartScreen"未知发布者"提示。要让 Windows 完全信任，证书须由受信任的公共 CA（如 DigiCert、GlobalSign、Let's Encrypt 不支持代码签名）签发；自签名或测试证书仅用于内部验证。

### 测试（默认无需管理员）

主包测试（`simulation_test.go` 的模拟异常）只操作内存数据，不读真实磁盘，普通终端即可运行：

```powershell
go test ./...
```

唯一例外：若你本机用 `build.bat` 构建过且残留了 `rsrc.syso`，该文件会让测试二进制也带上 `requireAdministrator` manifest，此时 `go test .` 会报 `The requested operation requires elevation`。`build.bat` 现在构建完会自动删除 `rsrc.syso`；老残留可手动删除：

```powershell
del rsrc.syso
```

`./health/ ./smart/ ./ui/` 三个子包不读真实磁盘，任何权限下都能跑。

### 日志文件

- 默认写入 exe 所在目录的 `smonitor.log`；
- 若目录不可写（如放在 `Program Files` 下），自动回退到 `%TEMP%\smonitor.log`。

## 采集方式

- **主路径 — IOCTL**：枚举 `PhysicalDrive0..255`，通过 `IOCTL_STORAGE_QUERY_PROPERTY` 读取型号/序列号/总线类型；ATA 使用 `IOCTL_SMART_RCV_DRIVE_DATA` 与 ATA pass-through，NVMe 使用 `IOCTL_STORAGE_PROTOCOL_COMMAND` 读取 Health Log Page 0x02。
- **兼容回退 — WMI**：对未读到属性或 ATA SMART 数据校验和异常的磁盘，通过 `root\WMI\MSStorageDriver_FailurePredictData` / `FailurePredictThresholds` 读取属性和阈值；已成功采集的数据不会被覆盖。
- **NVMe SSD**：`IOCTL_STORAGE_PROTOCOL_COMMAND`（Get Log Page 0x02，健康日志）。
- 不需要 smartctl.exe / 不需要任何外部依赖。

## 已验证的 CrystalDiskInfo 厂商兼容

以下规则均有 CrystalDiskInfo 源码依据；未列出的型号保留 ATA/NVMe 的通用解释，不会猜测性套用厂商字段。

| 型号范围 | 专用解释 |
|---|---|
| KIOXIA SATA SSD | `F1` 主机写入按 32 MB 单位；`AD` 显示健康度 |
| WD Blue SA510 | `F1/F2/E9` 直接以 GB 显示；`E6` 显示健康度（剩余寿命，低于 50% 严重） |
| 致态 / ZHITAI (YMTC) SATA | `F3` 为温度；`F1/F2` 按 512 B LBA 显示 |
| Samsung SATA SSD | `B1` 为剩余寿命；`F1/F2` 按 512 B LBA 显示；不作用于 Samsung HDD |
| Crucial MX/BX100/200/300/500 SATA | `F1/F2` 按 32 MB 单位显示 |
| Intel / Solidigm SATA | `F1` 主机写入、`F3` NAND 写入均按 32 MB 单位显示 |
| Seagate | `F1/F2` 主机读写与 `E9/EA` NAND 写入直接以 GB 显示 |
| Kingston KC600 | `F1/F2` 按 32 MB 单位显示；其他 Kingston 型号不套用此换算 |

`F3` 是典型厂商复用字段：YMTC 将其用作温度，而 Intel/Solidigm 将其用作 NAND 写入；未知型号只显示原始数值。

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
| 0xE9 Media_Wearout_Indicator | SSD 寿命 | 剩余寿命 < 50% | 严重 |
| 0xE8 Available_Reservd_Space | 预留块/寿命 | 剩余寿命 < 50% | 严重 |
| 0xAD / 0xB1 / 0xCA / 0xE6 / 0xE7 | 各厂牌剩余寿命字段 | 剩余寿命 < 50% | 严重 |

> 注：厂商只为少数关键属性设置设备阈值（KIOXIA 仅 `A9/C2`，WD Blue SA510 仅 `05/AD/B8/C2/E8` 等），这与 CrystalDiskInfo 完全一致；其余条目按 Raw/Value 动态规则判断，不表示"未检测"。表格新增"含义"列，每个属性都附中文说明。

### NVMe
| 字段 | 条件 | 严重度 |
|---|---|---|
| Media_Data_Integrity_Errors ("0E") | != 0 | 严重 |
| Percentage_Used | 已用 > 50%（剩余寿命 < 50%） | 严重 |
| Temperature（复合温度） | > 60°C | 严重 |
| Critical_Warning | != 0 | 严重 |
| Endurance_Group_Critical_Warning_Summary | != 0 | 严重 |
| Available_Spare < Threshold | | 严重 |
| Read_Only_Mode | != 0 | 严重 |

> 注：Temperature Sensor 1–8 是厂商自定义位置（控制器/NAND/PCB 等），NVMe 规范没有通用阈值，**仅展示不告警**（告警依据复合温度与 Critical Warning），避免误报。

ATA SMART 数据页校验和异常会作为数据完整性警告显示，并触发 WMI 回退；不会被当作“健康”。

## 严谨性说明

- **0x01 Read Error Rate 在 WD/Seagate 健康盘上 raw 也几乎必非零**（WD 是打包复合值，Seagate 低 32 位计可纠正 ECC），因此只按设备报告的归一化阈值判定。
- **0x0E / 0xBC 在 ATA 规范中未统一定义**；0x0E 不设默认告警，0xBC 的累计值超过 10 时作为兼容性警告显示。
- **0x05 与 0xBB** 是 ATA 失效的最可靠指标，任何非零均视为风险。
- **NVMe 路径**中的 "Media and Data Integrity Errors" 字段才是用户定义的 0E 语义在 NVMe 中的真正对应。
- **阈值少是正常的**：设备阈值由厂商写入 SMART 阈值页，多数 SSD 厂商只为少数属性设置阈值（与 CrystalDiskInfo 的 Thr 列一致），判断仍然覆盖所有关键属性。

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

在 GUI 中点击“模拟异常验证”即可直接查看主窗口的红黄表格、顶部异常横幅和异常复制格式。该操作只加载内存中的固定数据，不读取或写入真实硬盘；不会弹出独立告警窗；点击“重新扫描”会恢复真实磁盘数据。

下面的测试构造 ATA 重映射扇区、过热、命令超时、厂商 Pre-fail，以及 NVMe 临界位图、介质错误、传感器过热和寿命告警；同时验证“复制异常条目”的 TSV 内容不包含健康属性：

```powershell
go test -run TestSimulatedDiskFailuresProduceFeedbackReport -v .
```
