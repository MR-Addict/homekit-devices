# Chromecast 控制

[Go 总览](../README.md) · [HomeKit 配置](homekit.md) · [OpenWrt 部署](../deploy/openwrt/chromecast/README.md)

`homekit-chromecast` 通过 Android TV Remote v2 控制 Chromecast with Google TV：HomeKit 提供电视配件、遥控按键和可配置的应用输入源，CLI 提供遥控。开启为唤醒，关闭为待机，不重启或关闭 Android 系统。状态来自 Chromecast 自身，不代表电视屏幕状态；设备的 HDMI-CEC 设置可能影响电视。无需 ADB。

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

## HomeKit 电视遥控器

配对后家庭 App 应显示电视，可开关 Chromecast；在 iPhone/iPad 控制中心的 Apple TV 遥控器检查是否能选择该配件。支持方向、确认、返回、退出（Android HOME）、播放/暂停、快进/快退、上一项/下一项按键；遥控器右下角 `i` 按钮映射为 Android HOME（返回主页），实际显示的按键由 iOS 决定。扬声器服务提供相对音量增减和静音；静音按目标状态处理，设备未提供有效音量状态时拒绝盲目切换。音量和应用状态仍以设备上报为准。

普通按键不会主动唤醒设备。选择应用输入源时先确认 Chromecast 已唤醒，再发送启动命令；启动失败不会自动重试。当前输入源只根据上报的前台包名匹配，未知或不在列表内时标识为 `0`，不虚构已选中的应用。应用名称由 YAML 管理，在家庭 App 中只读。本版 HomeKit 不提供文本输入、语音或长按界面，仍可使用 CLI。

从旧开关版本升级时保留 Remote v2 凭据及 HomeKit 存储目录。如果家庭 App 仍显示开关，移除旧配件后使用原 PIN 重新添加；程序不会自动清除配对。控制中心的自定义电视遥控界面是否显示、具体按键及音量是否可用，需要在实际 iOS 设备上验证。

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

可选顶层 `apps` 定义应用输入源，省略或空列表不创建应用输入源。每项的 `id` 为稳定正整数（不要随列表顺序重新编号），`name` 为显示名称，`launch` 为应用 ID 或链接，`package` 为设备上报的前台应用包名；字段不能为空，ID 和包名不能重复。配置更改后重启服务。名称/启动目标改变时保留 ID，移除或添加输入源后在家庭 App 检查列表。

```yaml
apps:
  - id: 1
    name: "YouTube"
    launch: "https://www.youtube.com"
    package: "com.google.android.youtube.tv"
  - id: 2
    name: "哔哩哔哩"
    launch: "yst://com.xiaodianshi.tv.yst"
    package: "com.xiaodianshi.tv.yst"
```

部分 Google TV 的 Play Store 版本不再支持按包名启动；上述两个入口使用已实测的应用链接，`package` 仍用于识别前台应用。包名与链接需按实际安装应用填写；可用 `status`/`watch` 的 `app` 字段确认前台包名。

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

默认 `power_mode: toggle`：读取明确的新鲜状态，只在与目标不同时发送 POWER，再等待上报目标状态。未知状态拒绝切换，写入结果不明时不重发。`discrete` 使用 WAKEUP/SLEEP，仅在固件实测支持后配置；无自动 POWER 回退。HomeKit 电源写入异步接受，同目标请求合并、反向请求拒绝，最终值只来自观测；失联或操作失败时读取通信错误。启动与重连不会自动开关 Chromecast。

本机使用相同凭据路径的连接通过文件锁串行执行，可与服务并用；不同主机或不同凭据副本不共享锁，避免同时发送电源命令。每次遥控操作复用现有的单次 Remote v2 会话，会有连接和状态等待延迟。服务内的电源、遥控及轮询串行执行，等待和命令均受超时限制；重复按键不会合并，超时或发送失败不自动重试。长按或语音可能暂时阻塞服务轮询，超过状态新鲜度窗口时 HomeKit 返回通信错误。
