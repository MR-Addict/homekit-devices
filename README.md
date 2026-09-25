# homekit-devices

把自制 ESP32 设备接入 Apple HomeKit 的个人项目集合。基于 [HomeSpan](https://github.com/HomeSpan/HomeSpan) 实现，无需网关或桥接设备，ESP32 通过家庭局域网直接与 HomeKit 配对，即可在「家庭」App 和 Siri 中控制。

## 项目结构

```
src/
├── lamp/   台灯：HomeKit 开关 + 实体按键
└── wol/    网络唤醒：发送 Magic Packet 唤醒局域网内的电脑
```

每个子目录是一个独立的 Arduino sketch（目录名 = 主文件名），可分别编译烧录，互不影响。各设备的功能说明见对应子目录的 README。

命名约定：头文件与入口文件用小写（`<名称>.h` / `<名称>.ino`），设备服务类统一使用 `HS` 前缀（如 `HSWOL`、`HSTableLamp`）。

## 通用说明

### 依赖库

| 库 | 版本 | 用途 |
| --- | --- | --- |
| [HomeSpan](https://github.com/HomeSpan/HomeSpan) | 2.0.0+ | HomeKit 接入（需要 Arduino-ESP32 v3.x） |
| [WakeOnLan](https://github.com/a7md0/WakeOnLan) | 1.1.7 | 仅 `wol` 使用，发送唤醒包 |

### Flash 与分区方案

ESP32 芯片至少有 4MB flash，但程序实际可用空间由**分区方案**决定，而不是芯片总容量。HomeSpan 程序在 Arduino IDE 默认分区方案（App 仅约 1.3MB）下会超出，出现 *Sketch Too Big* 错误。

请在 Arduino IDE `工具 → 分区方案（Partition Scheme）` 中选择 **Minimal SPIFFS**：每个 OTA 分区 1.9MB，SPIFFS 只占 128KB（HomeSpan 不使用 SPIFFS）。

- 本项目启用了 OTA，分区方案必须包含 OTA 分区，因此不能使用 `Huge App`
- 分区方案**无法通过 OTA 修改**：首次切换需用 USB 连接电脑重新烧录一次，之后即可继续使用 OTA

### 配网与配对

1. 烧录后打开串口监视器（波特率 `115200`），按提示输入 WiFi 名称与密码
2. 在「家庭」App 中添加配件，扫描串口输出的配对二维码
3. 默认配对码 **466-37-726**，配对 PIN **HSPN**，可在串口 CLI 中修改

参考：[HomeSpan QRCodes](https://github.com/HomeSpan/HomeSpan/blob/master/docs/QRCodes.md)

### OTA

两个 sketch 均已启用 OTA（密码 `ota`）。设备联网后可在 Arduino IDE 中通过网络更新，无需 USB 连接。

## 烧录流程

1. 安装 Arduino IDE 与 Arduino-ESP32 v3.x
2. 安装依赖库（HomeSpan、WakeOnLan）
3. 选择 `工具 → 分区方案 → Minimal SPIFFS`
4. 打开 `src/<设备>/<设备>.ino`，编译并上传
5. 按上文完成配网与配对
