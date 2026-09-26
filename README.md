# homekit-devices

把自制 ESP32 设备接入 Apple HomeKit 的个人项目集合。基于 [HomeSpan](https://github.com/HomeSpan/HomeSpan) 实现，无需网关或桥接设备，ESP32 通过家庭局域网直接与 HomeKit 配对，即可在「家庭」App 和 Siri 中控制。

## 项目结构

```
src/
├── common/ 共用 BLE 配网服务及 OTA 密码
├── fan/    风扇：ESP32-C3 HomeKit 开关 + 实体按键 + BLE 配网，恢复上次状态
├── lamp/   台灯：HomeKit 开关 + 实体按键 + BLE 配网
└── wol/    网络唤醒：BLE 配网后发送 Magic Packet 唤醒局域网内的电脑
```

每个子目录是一个独立的 Arduino sketch（目录名 = 主文件名），可分别编译烧录，互不影响。各设备的功能说明见对应子目录的 README。

三种设备共用 `web/ble-provision/` 中的 BLE 配网页面；即使设备失去 Wi‑Fi，也可从附近电脑重新配置。

命名约定：头文件与入口文件用小写（`<名称>.h` / `<名称>.ino`），设备服务类统一使用 `HS` 前缀（如 `HSWOL`、`HSTableLamp`）。

## 通用说明

### 依赖库

| 库 | 版本 | 用途 |
| --- | --- | --- |
| [HomeSpan](https://github.com/HomeSpan/HomeSpan) | 2.0.0+ | HomeKit 接入（需要 Arduino-ESP32 v3.x） |
| [WakeOnLan](https://github.com/a7md0/WakeOnLan) | 1.1.7 | 仅 `wol` 使用，发送唤醒包 |
| [NimBLE-Arduino](https://github.com/h2zero/NimBLE-Arduino) | 2.5.1 | 三种设备共用的 BLE 配网服务 |

### Flash 与分区方案

ESP32 芯片至少有 4MB flash，但程序实际可用空间由**分区方案**决定，而不是芯片总容量。HomeSpan 程序在 Arduino IDE 默认分区方案（App 仅约 1.3MB）下会超出，出现 *Sketch Too Big* 错误。

请在 Arduino IDE `工具 → 分区方案（Partition Scheme）` 中选择 **Minimal SPIFFS**：每个 OTA 分区 1.9MB，SPIFFS 只占 128KB（HomeSpan 不使用 SPIFFS）。

- 本项目启用了 OTA，分区方案必须包含 OTA 分区，因此不能使用 `Huge App`
- 分区方案**无法通过 OTA 修改**：首次切换需用 USB 连接电脑重新烧录一次，之后即可继续使用 OTA

### 配网与配对

1. 烧录后可用下方 BLE 配网页面设置 Wi‑Fi；也可打开串口监视器（波特率 `115200`），按提示输入 Wi‑Fi 名称与密码
2. 在「家庭」App 中添加配件，扫描串口输出的配对二维码
3. 默认配对码 **466-37-726**，配对 PIN **HSPN**，可在串口 CLI 中修改

参考：[HomeSpan QRCodes](https://github.com/HomeSpan/HomeSpan/blob/master/docs/QRCodes.md)

### OTA

三个 sketch 均已启用 OTA（密码 `ota`）。设备联网后可在 Arduino IDE 中通过网络更新，无需 USB 连接。

### BLE 配网页面

在 `web/ble-provision/` 中运行 `npm ci` 和 `npm run dev`，然后用电脑上的 Chrome 或 Edge 打开 Vite 显示的本地地址。搜索并连接设备后，首次需验证 OTA 密码 `ota`，然后填写 Wi‑Fi 信息。验证成功的 OTA 密码会保存在当前浏览器的 localStorage 中，下次连接时自动验证。页面会在设备保存凭据并重启后尝试重新连接蓝牙，确认 Wi‑Fi 状态；连接后可随时手动断开蓝牙。设备名分别以 `HS-Fan-`、`HS-Lamp-`、`HS-WOL-` 开头，后四位用于区分设备。

推送到 `main` 后，GitHub Action 会将 `npm run build` 的产物发布到 [GitHub Pages](https://MR-Addict.github.io/homekit-devices/)；首次发布需要在仓库 Pages 设置中选择 `gh-pages` 分支作为发布源。Web Bluetooth 需要 HTTPS 或 localhost 环境；iPhone/iPad Safari 不原生支持。Wi‑Fi 信息不会保存在浏览器或上传到服务器；OTA 密码仅保存在当前浏览器。这里的 `ota` 密码已公开于代码中，只用于避免误操作。

## 烧录流程

1. 安装 Arduino IDE 与 Arduino-ESP32 v3.x；风扇选择 **ESP32C3 Dev Module** 开发板
2. 安装依赖库（HomeSpan、NimBLE-Arduino；`wol` 还需 WakeOnLan）
3. 选择 `工具 → 分区方案 → Minimal SPIFFS`
4. 打开 `src/<设备>/<设备>.ino`，编译并上传
5. 按上文完成配网与配对
