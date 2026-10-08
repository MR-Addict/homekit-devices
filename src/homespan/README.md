# HomeSpan 设备

基于 [HomeSpan](https://github.com/HomeSpan/HomeSpan) 的 ESP32 固件，通过家庭局域网直接接入 Apple HomeKit，可在「家庭」App 和 Siri 中控制。

| 设备 | 功能 | 专属说明 |
| --- | --- | --- |
| 风扇 | HomeKit 开关、实体按键、恢复上次状态 | [接线与配置](fan/README.md) |
| 台灯 | HomeKit 开关、实体按键 | [接线与配置](lamp/README.md) |
| 网络唤醒 | 向局域网电脑发送 Magic Packet | [目标设备配置](wol/README.md) |

每个设备子目录是独立 Arduino sketch，目录名与入口文件名一致，可分别编译烧录。三个设备共用 `common/` 中的 BLE 配网服务与 OTA 密码；页面使用及开发见 [BLE 配网页面](../web/ble-provision/README.md)。

## 依赖库

| 库 | 版本 | 用途 |
| --- | --- | --- |
| [HomeSpan](https://github.com/HomeSpan/HomeSpan) | 2.0.0+ | HomeKit 接入（需要 Arduino-ESP32 v3.x） |
| [WakeOnLan](https://github.com/a7md0/WakeOnLan) | 1.1.7 | 仅 `wol` 使用，发送唤醒包 |
| [NimBLE-Arduino](https://github.com/h2zero/NimBLE-Arduino) | 2.5.1 | 三种设备共用的 BLE 配网服务 |

## Flash 与分区方案

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

1. 烧录后按 [BLE 配网页面指南](../web/ble-provision/README.md) 设置 Wi-Fi；也可在串口监视器中按提示输入 Wi-Fi 名称与密码
2. 在「家庭」App 中添加配件，扫描串口输出的配对二维码
3. 默认配对码 **466-37-726**，配对 PIN **HSPN**，可在串口 CLI 中修改

参考：[HomeSpan QRCodes](https://github.com/HomeSpan/HomeSpan/blob/master/docs/QRCodes.md)

## OTA

三个 sketch 均已启用 OTA，默认密码 `ota`。联网后可在 Arduino IDE 中选择网络端口更新；首次烧录及分区方案变更使用 USB。
