# HomeSpan 设备

把自制 ESP32 设备接入 Apple HomeKit 的个人项目集合。基于 [HomeSpan](https://github.com/HomeSpan/HomeSpan) 实现，无需网关或桥接设备，ESP32 通过家庭局域网直接与 HomeKit 配对，即可在「家庭」App 和 Siri 中控制。

## 项目结构

```
src/homespan/
├── common/ 共用 BLE 配网服务及 OTA 密码
├── fan/    风扇：ESP32 HomeKit 开关 + 实体按键 + BLE 配网，恢复上次状态
├── lamp/   台灯：HomeKit 开关 + 实体按键 + BLE 配网
└── wol/    网络唤醒：BLE 配网后发送 Magic Packet 唤醒局域网内的电脑
```

每个设备子目录是一个独立的 Arduino sketch（目录名 = 主文件名），可分别编译烧录。功能、接线及设备专属设置见[风扇](fan/README.md)、[台灯](lamp/README.md)和[网络唤醒](wol/README.md)的说明。

三种设备共用 `src/web/ble-provision/` 中的 BLE 配网页面；即使设备失去 Wi‑Fi，也可从附近电脑重新配置。

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

## 烧录流程

1. 安装 Arduino IDE 与 Arduino-ESP32 v3.x，选择与实际硬件匹配的开发板
2. 安装依赖库（HomeSpan、NimBLE-Arduino；`wol` 还需 WakeOnLan）
3. 选择 `工具 → 分区方案 → Minimal SPIFFS`
4. 如果使用 ESP32-C3 的原生 USB 接口查看串口输出，在 `工具 → USB CDC On Boot` 中选择 **Enabled**；更改该设置后需重新编译上传。使用板载 USB 转串口芯片（如 CH340、CP210x）时保持 **Disabled**，并选择对应串口
5. 打开 `src/homespan/<设备>/<设备>.ino`，编译并上传
6. 在 Arduino IDE 中选择开发板当前的串口，以 `115200` 波特率打开串口监视器，按一下 **RESET/EN** 键查看启动日志。启用原生 USB CDC 后，串口名称可能变化，需要重新选择
7. 按下文完成配网与配对

## 配网与配对

1. 烧录后使用下方 BLE 配网页面设置 Wi‑Fi；也可在串口监视器中按提示输入 Wi‑Fi 名称与密码
2. 在「家庭」App 中添加配件，扫描串口输出的配对二维码
3. 默认配对码 **466-37-726**，配对 PIN **HSPN**，可在串口 CLI 中修改

参考：[HomeSpan QRCodes](https://github.com/HomeSpan/HomeSpan/blob/master/docs/QRCodes.md)

### BLE 配网页面

在 `src/web/ble-provision/` 中运行：

```sh
pnpm install --frozen-lockfile
pnpm dev
```

用电脑上的 Chrome 或 Edge 打开 Vite 显示的本地地址，或打开 [GitHub Pages](https://MR-Addict.github.io/homekit-devices/)。搜索并连接设备后，首次需验证 OTA 密码 `ota`。设备名分别以 `HS-Fan-`、`HS-Lamp-`、`HS-WOL-` 开头，后四位用于区分设备。新版固件会在验证后提供已保存的 Wi‑Fi 凭据供页面回填；旧固件仍可手动填写。SSID 最多 32 个 UTF‑8 字节，密码最多 64 个 UTF‑8 字节；开放网络的密码可以留空。输错凭据时，可重新连接设备并修改。

设备确认保存凭据后，页面立即显示设置完成；设备随后自动重启并尝试连接 Wi‑Fi。完成提示不表示设备已连上网络。验证成功的 OTA 密码保存在当前浏览器的 localStorage 中，供下次连接时自动验证；Wi‑Fi 凭据只在当前页面会话中回填，不会保存在浏览器或上传到服务器。

Web Bluetooth 需要 HTTPS 或 localhost 环境；iPhone/iPad Safari 不原生支持。推送到 `main` 后，GitHub Action 使用 `pnpm build` 将页面发布到 [GitHub Pages](https://MR-Addict.github.io/homekit-devices/)；首次发布需要在仓库 Pages 设置中选择 `gh-pages` 分支作为发布源。

BLE 服务通过应用层 OTA 密码验证操作，但当前特征值未要求 BLE 配对或加密连接。密码 `ota` 已公开于代码和文档，只能避免误操作，不能阻止附近知晓密码的人读取或修改 Wi‑Fi 凭据。

## OTA

三个 sketch 均已启用 OTA（密码 `ota`）。设备联网后可在 Arduino IDE 中通过网络更新，无需 USB 连接。
