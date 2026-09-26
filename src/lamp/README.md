# 台灯（lamp）

ESP32 台灯控制，接入 Apple HomeKit。可在「家庭」App 中开关灯，也可通过实体按键本地翻转状态。

固件会广播 `HS-Lamp-xxxx` BLE 配网服务。操作方法见[通用 BLE 配网指南](../../README.md#ble-配网页面)。

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
