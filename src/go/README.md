# Go HomeKit 服务

一个 Go 模块提供两个独立程序：

- `homekit-wol`：HomeKit 桥接服务；普通设备打开时发送 Magic Packet、两秒后复位；配置 PVE 后显示检测状态，支持 WOL 开机与正常关机。
- `homekit-temperature`：独立 HomeKit 温度配件，读取路由器 Linux thermal 传感器，默认每 30 秒更新。

依赖 Go 1.25.0 或更新版本及 `patch` 命令。以下命令均从 `src/go/` 运行。首次开发或补丁更新后先准备依赖，之后正常使用 Go 命令：

```sh
./scripts/prepare-hap.sh
```

仓库仅保存 HAP 小补丁，完整源码生成到被忽略的 `third_party/hap/`。OpenWrt 构建脚本自动准备依赖；详见 [补丁说明](patches/README.md)。

## 架构

`cmd/` 仅负责命令行与组装。`internal/hapserver/` 共享配置、配对存储、监听接口与生命周期；`internal/wol/` 和 `internal/temperature/` 分别负责功能配置与配件。新增 Go 功能时在此模块内新增包及入口，复用共享包，无需复制服务模板或新增 `go.mod`。

## WOL

```sh
cp -n deploy/openwrt/wol/config.example.yaml deploy/openwrt/wol/config.yaml
# 本地运行时修改 storage_path、监听地址及 interfaces
go run ./cmd/homekit-wol -config deploy/openwrt/wol/config.yaml
```

至少配置一个 `devices` 条目。目标电脑需启用 BIOS 和网卡的 WOL。默认桥接名 `Wake Targets`，PIN `001-02-003`，配对数据 `./db`，广播地址 `255.255.255.255`，UDP 端口 `9`。设备可单独覆盖 `broadcast_ip` 和 `port`；建议使用目标 LAN 的子网广播地址。

使用默认有限广播地址时，还会尝试活动 IPv4 接口的广播地址。MAC 必须是标准六字节地址，名称和 MAC 不可重复。旧版单设备 `device:` 配置需要改成 `devices:`；旧单设备服务升级为桥接可能需要重新配对。

本次合并保留原 WOL 的默认值和配件身份算法。已有多设备部署升级时，沿用原配置和 `db` 数据，并保持工作目录或设置绝对 `homekit.storage_path`。相对存储路径始终相对于进程工作目录。

### PVE 电源控制

在设备条目下添加：

```yaml
options:
  type: pve
  host: "192.0.2.10"
  token: "<用户>@pve!<令牌名>=<密钥>"
```

`host` 必须是主机固定 IPv4 地址，使用 HTTPS 8006。按首版约定跳过服务器证书验证，无需证书文件；Token 负责认证。通过 `/api2/json/nodes` 自动发现唯一节点并缓存，多节点或发现失败明确报错；`name` 只用于显示，不参与 API 节点路径。专用用户及权限分离 Token 均在目标 `/nodes/<node>` 配置 `Sys.Audit`、`Sys.PowerMgmt`，不需要 VM 管理权限。

每 5 秒查询节点状态，单次请求超时 3 秒。API 成功表示运行；API 返回错误或 ICMP 可达表示主机在线但管理异常，此时读取返回 HomeKit 通信错误、日志记录故障。API 无响应且连续三轮 ICMP 无响应才推定离线；探测自身失败不判关机。首次状态未知时读取不可用，但允许发送开机命令。该离线判断可能把断网推定为关机。

点击开关只发起命令，保持检测到的值，开机后 API 恢复才亮，关机后检测离线才灭。关机使用 PVE 正常宿主机关机流程，来宾处理遵循现有配置。等待最多 5 分钟；同方向重复请求合并，过渡期间反向请求被拒绝，超时记录错误并继续检测，不自动重复关机。HomeKit 客户端对通信错误及过渡显示的呈现可能不同。

`patches/hap-v0.0.35.patch` 基于 HAP v0.0.35，增加命令写入钩子以保留检测状态，包含同值写入；其他配件保持原有行为。生成的 `third_party/hap/` 不进入仓库。配件身份和配对数据不因启用 PVE 而改变。Token 只放被忽略的实际配置，设置 `chmod 600`，不要提交或输出到日志。

## 路由器温度

```sh
go run ./cmd/homekit-temperature -list-sensors
cp -n deploy/openwrt/temperature/config.example.yaml deploy/openwrt/temperature/config.yaml
# 本地运行时修改 storage_path、监听地址及 interfaces
go run ./cmd/homekit-temperature -config deploy/openwrt/temperature/config.yaml
```

`-list-sensors` 无需配置文件，也不会启动 HomeKit。Linux thermal zone 的 `type` 帮助识别传感器；不能假定 `thermal_zone0` 就是 CPU。默认配件名称为“路由器温度”，PIN `001-02-003`，配对数据 `./temperature-db`。可配置 `temperature.name`、必填的 `temperature.path` 和正数时长 `temperature.interval`（默认 `30s`）。

读数按整数千分之一摄氏度转换，支持 HomeKit 默认范围 0–100°C。缺失、无效、超范围或首次不可读的读数导致明确错误；运行中失败保留最后有效温度并设置 `StatusFault`，恢复后清除。Apple Home 对故障的具体显示取决于客户端。名称改变不改变默认序列号或配件 ID；多台路由器可分别设置 `homekit.serial_number`。

## 通用 HomeKit 配置

两个配置均支持 `homekit.name`、`pin`、`storage_path`、`listen_address`、`interfaces`、`serial_number`、`manufacturer`、`model`、`firmware`。PIN 可使用八位或 `3-2-3` 格式；监听地址为 `host:port`。同时运行时使用不同端口、不同配对目录，并分别添加到 Apple Home。

## 构建与检查

```sh
./scripts/prepare-hap.sh
go test ./...
go vet ./...
mkdir -p bin
go build -o bin/homekit-wol ./cmd/homekit-wol
go build -o bin/homekit-temperature ./cmd/homekit-temperature
./scripts/build-openwrt.sh
```

部署目录按 `wol/` 和 `temperature/` 分组，仅跟踪 `config.example.yaml`。复制为同目录 `config.yaml` 后填写实际信息；实际配置被 Git 忽略，程序缺少配置会报错，不会回退到示例。

OpenWrt 脚本输出 `bin/homekit-wol-linux-arm64` 和 `bin/homekit-temperature-linux-arm64`，关闭 CGO。温度服务运行时需要 Linux thermal sysfs；本地测试使用临时文件。其他 WOL 平台可自行设置 `GOOS`、`GOARCH` 构建，例如 Windows 或 Linux MIPS little-endian（`GOMIPS=softfloat`）；Raspberry Pi Zero 使用 ARM 架构，不是 MIPS。

安装、启停、升级及实机验收见 [OpenWrt 部署](deploy/openwrt/README.md)。
