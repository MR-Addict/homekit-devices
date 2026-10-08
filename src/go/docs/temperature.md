# 温度传感器

[Go 总览](../README.md) · [HomeKit 配置](homekit.md) · [OpenWrt 部署](../deploy/openwrt/temperature/README.md)

`homekit-temperature` 提供独立 HomeKit 温度配件，读取 Linux thermal sysfs，默认每 30 秒更新。

## 本地运行

先按 Go 总览准备依赖。以下命令从 `src/go/` 执行，传感器枚举应在实际运行服务的 Linux 设备上进行：

```sh
go run ./cmd/homekit-temperature -list-sensors
cp -n deploy/openwrt/temperature/config.example.yaml deploy/openwrt/temperature/config.yaml
# 填写实际传感器路径，并按本地环境修改 HomeKit 存储、监听地址和接口。
go run ./cmd/homekit-temperature -config deploy/openwrt/temperature/config.yaml
```

`-list-sensors` 无需配置，也不会启动 HomeKit。通过各 thermal zone 的 `type` 识别传感器，不能假定 `thermal_zone0` 就是 CPU。

## 专属配置

| `temperature` 字段 | 说明 |
| --- | --- |
| `name` | 传感器名称，默认“路由器温度” |
| `path` | 必填，实际 thermal zone 的 `temp` 文件路径 |
| `interval` | 正数时长，默认 `30s` |

读数为整数千分之一摄氏度，转换后支持 HomeKit 默认范围 0–100°C。缺失、无效、超范围或首次不可读的读数会报错；运行中读取失败保留最后有效温度并设置 `StatusFault`，恢复后清除故障。Apple Home 对故障的具体显示取决于客户端。

修改名称不会改变默认序列号或配件 ID；多实例应分别设置 `homekit.serial_number`。
