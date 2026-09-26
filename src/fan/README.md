# 风扇（fan）

ESP32-C3 风扇开关，接入 Apple HomeKit。可在「家庭」App 中开关，也可用实体按键切换。开关状态保存在 HomeSpan 的 NVS 中，断电重启后恢复上次状态；首次启动默认关闭。不提供调速功能。固件还提供 BLE 配网：即使没有 Wi‑Fi 连接，也能从附近的电脑修改 Wi‑Fi 凭据。

## 引脚与接线

| 功能 | GPIO | 接线与电平 |
| --- | --- | --- |
| 风扇开关信号 | 4 | 高电平开启，低电平关闭；连接风扇驱动电路的控制输入 |
| 手动按键 | 3 | 按键另一端接 GND；使用内部上拉，短按切换开关 |

GPIO 只能输出控制信号，**不要将风扇电机直接接到 GPIO4**。根据风扇的电压和电流选择合适的驱动电路及电源；建议在驱动输入端设置下拉，避免 ESP32-C3 启动时输入悬空。

## 编译与烧录

在 Arduino IDE 中选择 **ESP32C3 Dev Module**，分区方案选择 **Minimal SPIFFS**（保留 OTA 分区），打开 `fan.ino` 编译并上传。如果开发板通过 ESP32-C3 的**原生 USB**接口连接电脑，还需在「工具 → USB CDC On Boot」选择 **Enabled**，然后**重新编译并上传**；否则 `Serial` 不会输出到该 USB 串口。使用板载 USB 转串口芯片（如 CH340、CP210x）的开发板则保持 **Disabled**，并选择其对应串口。

上传后在 Arduino IDE 选择开发板当前的串口，打开串口监视器，设置波特率为 **115200**，再按一下开发板的 **RESET/EN** 键查看启动日志。原生 USB 开启 CDC 后，串口名称可能变化，需要重新选择。首次烧录后按项目总 README 的说明完成配网与 HomeKit 配对。OTA 密码为 `ota`。

## BLE 配网

如果设备正在运行旧版风扇固件，**先在旧 Wi‑Fi 仍可用时通过 OTA 上传新版固件**，或用 USB 烧录一次。以后即使设备无法连接 Wi‑Fi，也可以用 BLE 修改凭据。设备持续广播 `HS-Fan-xxxx`，其中后四位用于区分附近的风扇设备。

在 `web/ble-provision/` 目录运行：

```sh
npm ci
npm run dev
```

在电脑的 Chrome 或 Edge 中打开 Vite 显示的本地地址，或打开项目的 [GitHub Pages](https://MR-Addict.github.io/homekit-devices/)。点击“搜索设备”并选择 `HS-Fan-xxxx`，首次输入 OTA 密码 `ota` 验证，再填写 Wi‑Fi 名称及密码并提交。验证成功的 OTA 密码会保存在当前浏览器，下次连接时自动验证。网页会在设备保存凭据并重启后尝试重新连接 BLE，确认 Wi‑Fi 状态；也可随时手动断开。SSID 最多 32 个 UTF‑8 字节，密码最多 64 个 UTF‑8 字节；开放网络的密码可以留空。输错 Wi‑Fi 凭据时，设备仍会广播 BLE，可重新连接并填写正确配置。

Web Bluetooth 必须从浏览器支持的安全上下文使用；`localhost` 可用于本地运行。首版在电脑 Chrome/Edge 验证；若要在 Android Chrome 上打开，需要把静态页面放到可信的 HTTPS 地址。iPhone/iPad 的 Safari 不原生支持 Web Bluetooth。

BLE 写入要求加密连接，配网密码与本 sketch 的 OTA 密码相同，均为固定的 `ota`。**该密码已出现在项目代码和文档中，只能防止误操作，不能防止附近知晓密码的人修改设备的 Wi‑Fi。**页面会在验证成功后把 OTA 密码保存到当前浏览器的 localStorage；Wi‑Fi 信息不会保存在浏览器或上传到服务器。

## 文件

- `fan.ino`：入口，初始化 HomeSpan 与风扇配件
- `fan.h`：`HSFan` 服务（`Service::Fan`），处理开关状态、NVS 恢复与按键事件
- `../common/ble_provision.h`：三种设备共用的 BLE 配网服务、分包协议、OTA 密码与 HomeSpan 凭据写入
- `../../web/ble-provision/`：通用 Web Bluetooth 配网页面
