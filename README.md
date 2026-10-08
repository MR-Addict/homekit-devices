# homekit-devices

自制 HomeKit 设备与服务集合，在 Apple「家庭」App 和 Siri 中控制设备、查看传感器数据。

| 项目 | 功能 | 运行环境 | 文档 |
| --- | --- | --- | --- |
| HomeSpan | 风扇、台灯、网络唤醒 | ESP32 / Arduino | [固件与烧录](src/homespan/README.md) |
| Go | 多设备网络唤醒、PVE 电源控制、温度传感器、Chromecast 遥控 | Linux / OpenWrt；WOL 可运行于其他 Go 平台 | [服务与开发](src/go/README.md) · [OpenWrt 部署](src/go/deploy/openwrt/README.md) |
| BLE 配网页面 | 为 HomeSpan 设备设置 Wi-Fi | 支持 Web Bluetooth 的桌面浏览器 | [使用与开发](src/web/ble-provision/README.md) · [在线页面](https://MR-Addict.github.io/homekit-devices/) |

HomeSpan 的每个设备目录是独立 Arduino sketch。Go 的三个程序分别运行、配对和保存数据，使用 YAML 配置。
