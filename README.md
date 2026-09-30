# homekit-devices

自制 HomeKit 设备与服务集合，在 Apple「家庭」App 和 Siri 中控制设备、查看传感器数据。

| 实现 | 功能 | 运行环境 | 文档 |
| --- | --- | --- | --- |
| HomeSpan | 风扇、台灯、网络唤醒 | ESP32 / Arduino | [HomeSpan](src/homespan/README.md) |
| Go | 多设备网络唤醒、路由器温度 | Linux、OpenWrt；WOL 也支持其他 Go 平台 | [Go](src/go/README.md) |
| Web | HomeSpan 设备 BLE 配网 | 支持 Web Bluetooth 的桌面浏览器 | [配网使用说明](src/homespan/README.md#ble-配网页面) |

```text
src/
├── homespan/             ESP32 固件
│   ├── common/           共用 BLE 配网服务
│   ├── fan/
│   ├── lamp/
│   └── wol/
├── go/                   单 Go 模块，两个独立 HomeKit 服务
│   ├── cmd/              homekit-wol、homekit-temperature
│   ├── internal/         hapserver、wol、temperature
│   ├── scripts/          构建脚本
│   └── deploy/openwrt/   按应用组织的生产配置与 procd 服务
└── web/
    └── ble-provision/    BLE 配网页面
```

HomeSpan 的每个设备目录是独立 Arduino sketch。Go 的两个程序共享 HomeKit 基础包，但分别运行、配对和保存数据。BLE 配网页面用于 HomeSpan 设备；Go 服务使用 YAML 配置。

OpenWrt 首个目标设备是 GL.iNet GL-BE3600，部署方式为 ARM64 二进制与独立服务脚本，见 [OpenWrt 部署说明](src/go/deploy/openwrt/README.md)。

BLE 配网页面在 [GitHub Pages](https://MR-Addict.github.io/homekit-devices/) 发布，开发与测试在 `src/web/ble-provision/` 内运行。
