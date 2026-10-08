# Chromecast OpenWrt 部署

[通用安装与维护](../README.md) · [应用使用与配置](../../../docs/chromecast.md)

## 配置准备

按通用流程选择 `app='chromecast'`，复制本目录 `config.example.yaml` 为 `config.yaml`。填写 Chromecast 实际地址及 LAN 接口，建议通过 DHCP 静态租约固定设备地址。

示例端口为 `32044`，配对数据库为 `/etc/homekit-chromecast/db`，Remote v2 凭据存于 `/etc/homekit-chromecast/credentials/`。电源模式及 CLI 用法见应用文档。

## Remote v2 配对与启动

按通用流程安装文件后，在路由器创建凭据目录，再从本地开启交互式配对。退出路由器 shell 后，以下本地命令使用通用流程中已设置的 `router`：

```sh
ssh "$router" 'mkdir -p /etc/homekit-chromecast/credentials; chmod 700 /etc/homekit-chromecast/credentials'
ssh -t "$router" '/usr/bin/homekit-chromecast -config /etc/homekit-chromecast/config.yaml pair'
# 输入 Chromecast 显示的六位配对码。
ssh "$router" '/usr/bin/homekit-chromecast -config /etc/homekit-chromecast/config.yaml status'
```

已有身份可按应用文档复制证书与私钥，无需重新配对。确认 `status` 成功后，按通用流程启用并启动服务，再用配置 PIN 添加到 Apple Home。Remote v2 和 HomeKit 是两次独立配对。

升级或 sysupgrade 时同时保留数据库和 credentials，目录包含私钥；权限与备份方法见通用流程。

## 验收

- 完成三轮待机与唤醒，重复同方向命令不造成反向切换。
- 使用实体遥控器操作后，HomeKit 状态同步；在家庭 App 验证电视电源与状态通知。
- 在 iPhone 控制中心的 Apple TV 遥控器选择 Chromecast，验证连续方向/确认/返回、播放暂停和音量操作。
- 配置 `apps` 后，验证应用输入源选择会先唤醒再启动，当前输入源随前台应用变化。旧配对仍显示开关时移除并重新添加；保留 Remote v2 凭据和 HomeKit 存储。
- 服务重启后两类配对保留，网络中断时报告通信故障，恢复后重新同步状态。
- 在设备上核对按键、文本输入、应用启动、音量和语音的实际效果，命令发送成功不能替代效果验证。
