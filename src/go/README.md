# Go HomeKit 服务

一个 Go 模块提供两个独立程序：

- `homekit-wol`：HomeKit 桥接服务，每台目标电脑显示为一个开关；打开时发送 Magic Packet，两秒后复位。
- `homekit-temperature`：独立 HomeKit 温度配件，读取路由器 Linux thermal 传感器，默认每 30 秒更新。

依赖 Go 1.25.0 或更新版本。以下命令均从 `src/go/` 运行。

## 架构

`cmd/` 仅负责命令行与组装。`internal/hapserver/` 共享配置、配对存储、监听接口与生命周期；`internal/wol/` 和 `internal/temperature/` 分别负责功能配置与配件。新增 Go 功能时在此模块内新增包及入口，复用共享包，无需复制服务模板或新增 `go.mod`。

## WOL

```sh
cp deploy/openwrt/wol/wol.config.yaml config.yaml
# 本地运行时修改 storage_path、监听地址及 interfaces
go run ./cmd/homekit-wol -config config.yaml
```

至少配置一个 `devices` 条目。目标电脑需启用 BIOS 和网卡的 WOL。默认桥接名 `Wake Targets`，PIN `001-02-003`，配对数据 `./db`，广播地址 `255.255.255.255`，UDP 端口 `9`。设备可单独覆盖 `broadcast_ip` 和 `port`；建议使用目标 LAN 的子网广播地址。

使用默认有限广播地址时，还会尝试活动 IPv4 接口的广播地址。MAC 必须是标准六字节地址，名称和 MAC 不可重复。旧版单设备 `device:` 配置需要改成 `devices:`；旧单设备服务升级为桥接可能需要重新配对。

本次合并保留原 WOL 的默认值和配件身份算法。已有多设备部署升级时，沿用原配置和 `db` 数据，并保持工作目录或设置绝对 `homekit.storage_path`。相对存储路径始终相对于进程工作目录。

## 路由器温度

```sh
go run ./cmd/homekit-temperature -list-sensors
cp deploy/openwrt/temperature/temperature.config.yaml temperature.config.yaml
# 本地运行时修改 storage_path、监听地址及 interfaces
go run ./cmd/homekit-temperature -config temperature.config.yaml
```

`-list-sensors` 无需配置文件，也不会启动 HomeKit。Linux thermal zone 的 `type` 帮助识别传感器；不能假定 `thermal_zone0` 就是 CPU。默认配件名称为“路由器温度”，PIN `001-02-003`，配对数据 `./temperature-db`。可配置 `temperature.name`、必填的 `temperature.path` 和正数时长 `temperature.interval`（默认 `30s`）。

读数按整数千分之一摄氏度转换，支持 HomeKit 默认范围 0–100°C。缺失、无效、超范围或首次不可读的读数导致明确错误；运行中失败保留最后有效温度并设置 `StatusFault`，恢复后清除。Apple Home 对故障的具体显示取决于客户端。名称改变不改变默认序列号或配件 ID；多台路由器可分别设置 `homekit.serial_number`。

## 通用 HomeKit 配置

两个配置均支持 `homekit.name`、`pin`、`storage_path`、`listen_address`、`interfaces`、`serial_number`、`manufacturer`、`model`、`firmware`。PIN 可使用八位或 `3-2-3` 格式；监听地址为 `host:port`。同时运行时使用不同端口、不同配对目录，并分别添加到 Apple Home。

## 构建与检查

```sh
go test ./...
go vet ./...
mkdir -p bin
go build -o bin/homekit-wol ./cmd/homekit-wol
go build -o bin/homekit-temperature ./cmd/homekit-temperature
./scripts/build-openwrt.sh
```

部署目录按 `wol/` 和 `temperature/` 分组，配置为 BE3600 实际配置，WOL 目标是 PVE主机（`E0:51:D8:11:3D:CE`）。

OpenWrt 脚本输出 `bin/homekit-wol-linux-arm64` 和 `bin/homekit-temperature-linux-arm64`，关闭 CGO。温度服务运行时需要 Linux thermal sysfs；本地测试使用临时文件。其他 WOL 平台可自行设置 `GOOS`、`GOARCH` 构建，例如 Windows 或 Linux MIPS little-endian（`GOMIPS=softfloat`）；Raspberry Pi Zero 使用 ARM 架构，不是 MIPS。

安装、启停、升级及实机验收见 [OpenWrt 部署](deploy/openwrt/README.md)。
