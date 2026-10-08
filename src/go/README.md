# Go HomeKit 服务

一个 Go 模块提供三个独立程序，分别运行、配对和保存数据。

| 程序 | 功能 | 使用说明 | OpenWrt 部署 |
| --- | --- | --- | --- |
| `homekit-wol` | 多设备 WOL；可选 PVE 状态检测与正常关机 | [WOL / PVE](docs/wol.md) | [WOL 部署](deploy/openwrt/wol/README.md) |
| `homekit-temperature` | 读取 Linux thermal 温度传感器 | [温度传感器](docs/temperature.md) | [温度部署](deploy/openwrt/temperature/README.md) |
| `homekit-chromecast` | Chromecast HomeKit 电视遥控器与 Remote v2 CLI | [Chromecast](docs/chromecast.md) | [Chromecast 部署](deploy/openwrt/chromecast/README.md) |

共享字段与路径规则见 [HomeKit 配置](docs/homekit.md)，安装和维护流程见 [OpenWrt 部署](deploy/openwrt/README.md)。

## 开发准备

需要 Go 1.25.0 或更新版本及 `patch` 命令。以下命令均从 `src/go/` 执行。首次开发或补丁更新后先准备依赖：

```sh
./scripts/prepare-hap.sh
./scripts/prepare-atvremote.sh
```

仓库维护固定版本依赖的小补丁，完整源码生成到被忽略的 `third_party/`；不要手动编辑生成源码。OpenWrt 构建脚本自动准备依赖，详见 [补丁维护](patches/README.md)。

## 架构

`cmd/` 负责命令行与组装；`internal/hapserver/` 共享配置、配对存储、监听接口与生命周期；`internal/wol/`、`internal/temperature/` 和 `internal/chromecast/` 实现各应用。新增功能在本模块内增加包及入口，复用共享包。

## 构建与检查

完成依赖准备后执行：

```sh
go test ./...
go vet ./...
mkdir -p bin
go build -o bin/homekit-wol ./cmd/homekit-wol
go build -o bin/homekit-temperature ./cmd/homekit-temperature
go build -o bin/homekit-chromecast ./cmd/homekit-chromecast
./scripts/build-openwrt.sh
```

OpenWrt 脚本以 `GOOS=linux GOARCH=arm64 CGO_ENABLED=0` 构建三个程序，输出到 `bin/homekit-<应用>-linux-arm64`。其他 WOL 平台可自行设置 `GOOS`、`GOARCH`；Linux MIPS little-endian 可设置 `GOMIPS=softfloat`。温度服务运行时需要 Linux thermal sysfs，本地测试使用临时文件。
