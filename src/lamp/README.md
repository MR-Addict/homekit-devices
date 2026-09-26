# 台灯（lamp）

ESP32 台灯控制，接入 Apple HomeKit。可在「家庭」App 中开关灯，也可通过实体按键本地翻转状态。

固件会广播 `HS-Lamp-xxxx` BLE 配网服务。使用项目的通用配网页面连接设备，首次验证 OTA 密码 `ota` 后设置 Wi‑Fi；验证成功的密码会保存在当前浏览器，后续连接时自动验证。页面的本地运行与 GitHub Pages 地址见项目总 README。

## 引脚

| 功能 | GPIO |
| --- | --- |
| 驱动引脚 A | 25 |
| 驱动引脚 B | 26 |
| 驱动引脚 C | 27 |
| 按键 | 15 |

修改 `lamp.ino` 中 `new HSTableLamp(25, 26, 27, 15)` 的参数即可适配自己的接线。

## 文件

- `lamp.ino`：入口，初始化 HomeSpan 与配件
- `lamp.h`：`HSTableLamp` 服务（`Service::LightBulb`），处理开关状态与按键事件
- `../common/ble_provision.h`：与其他设备共用的 BLE 配网服务
