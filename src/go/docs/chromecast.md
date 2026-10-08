# Chromecast 控制

[Go 总览](../README.md) · [HomeKit 配置](homekit.md) · [OpenWrt 部署](../deploy/openwrt/chromecast/README.md)

`homekit-chromecast` 通过 Android TV Remote v2 控制 Chromecast with Google TV：HomeKit 提供电源开关，CLI 提供遥控。开启为唤醒，关闭为待机，不重启或关闭 Android 系统。状态来自 Chromecast 自身，不代表电视屏幕状态；设备的 HDMI-CEC 设置可能影响电视。无需 ADB。

## 本地运行与配对

先按 Go 总览准备依赖。从 `src/go/` 执行：

```sh
cp -n deploy/openwrt/chromecast/config.example.yaml deploy/openwrt/chromecast/config.yaml
# 填写设备地址，并按本地环境修改接口、存储目录和凭据路径。
mkdir -p bin
go build -o bin/homekit-chromecast ./cmd/homekit-chromecast
bin/homekit-chromecast -config deploy/openwrt/chromecast/config.yaml pair
bin/homekit-chromecast -config deploy/openwrt/chromecast/config.yaml status
bin/homekit-chromecast -config deploy/openwrt/chromecast/config.yaml serve
```

`pair` 提示输入设备显示的六位配对码，最多等待两分钟，超时可重新运行。只有显式 `pair` 创建 Remote v2 身份，启动不会生成或轮换凭据。已有身份可复制 `cert.pem` 和 `key.pem` 到配置路径，私钥权限设为 `0600`、目录为 `0700`，不要提交私钥。

Remote v2 配对与 Apple Home 配对相互独立；服务启动后在家庭 App 使用配置 PIN 添加配件。

## 专属配置

配置使用严格 YAML，只允许一个文档。`discover`、`keys` 无需配置；其余命令读取配置。`-config` 放在命令前，省略命令时运行 `serve`。

| 字段 | 说明与程序默认值 |
| --- | --- |
| `chromecast.host` | 必填，设备 IP 或主机名，不包含端口；建议设置 DHCP 静态租约 |
| `chromecast.api_port` | 可选 Remote v2 端口，省略或 `0` 使用协议默认端口 |
| `chromecast.cert_path` | 默认 `./chromecast-credentials/cert.pem` |
| `chromecast.key_path` | 默认 `./chromecast-credentials/key.pem`，不能与证书路径相同 |
| `chromecast.power_mode` | 默认 `toggle`，也支持 `discrete` |
| `control.interval` | 状态轮询间隔，默认 `5s` |
| `control.timeout` | 单次操作超时，默认 `5s` |
| `control.transition_timeout` | 电源转换超时，默认 `15s` |

三个时长均须为正数。相对凭据和存储路径以配置文件所在目录为基准，详见通用配置。

## 控制和状态

将构建后的程序加入 PATH，或将以下命令中的 `homekit-chromecast` 替换为 `bin/homekit-chromecast`；`config.yaml` 替换为实际配置路径。

```sh
homekit-chromecast -config config.yaml on
homekit-chromecast -config config.yaml off
homekit-chromecast -config config.yaml status
homekit-chromecast -config config.yaml watch
homekit-chromecast discover
homekit-chromecast keys
homekit-chromecast -config config.yaml key DPAD_CENTER
homekit-chromecast -config config.yaml key -hold 2s DPAD_DOWN
homekit-chromecast -config config.yaml text '搜索文本'
homekit-chromecast -config config.yaml launch com.google.android.youtube.tv
homekit-chromecast -config config.yaml volume up 2
homekit-chromecast -config config.yaml mute
homekit-chromecast -config config.yaml voice audio.pcm
```

`key -hold` 放在按键名之前，最大 30 秒；取消会尝试释放按键，断线时不能保证设备收到释放。`text` 等待当前连接收到输入字段信息；需先在 Chromecast 打开输入框。`launch` 支持应用 ID 和应用链接，实际响应依应用而定。语音读取原始 8 kHz、单声道、16 位小端 PCM，最长 60 秒，文件名 `-` 表示 stdin；不采集麦克风、不转码，不接受 WAV。CLI 不是 Cast 媒体发送器，播放/暂停使用遥控按键。

JSON `on` 是 Chromecast Remote v2 观测的电源状态。未知为 null，不把断线推断为待机。应用/音量信息可能尚未上报，音量字段也可能反映遥控服务的占位值；命令发送成功不等于应用或音量已经产生效果。未协商支持的功能明确报错。

默认 `power_mode: toggle`：读取明确的新鲜状态，只在与目标不同时发送 POWER，再等待上报目标状态。未知状态拒绝切换，写入结果不明时不重发。`discrete` 使用 WAKEUP/SLEEP，仅在固件实测支持后配置；无自动 POWER 回退。HomeKit 写入异步接受，同目标请求合并、反向请求拒绝，最终值只来自观测；失联或操作失败时读取通信错误。启动与重连不会自动开关 Chromecast。

本机使用相同凭据路径的连接通过文件锁串行执行，可与服务并用；不同主机或不同凭据副本不共享锁，避免同时发送电源命令。长按或语音可能暂时阻塞服务轮询，超过状态新鲜度窗口时 HomeKit 返回通信错误。
