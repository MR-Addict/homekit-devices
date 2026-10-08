# OpenWrt 部署

首个目标为 GL.iNet GL-BE3600 / aarch64，路由器通过 `ssh be3600` 访问。仓库只保存配置示例；实际配置由用户填写，Git 忽略 `config.yaml`。

```text
openwrt/
├── README.md
├── temperature/
│   ├── homekit-temperature.init
│   └── config.example.yaml
└── wol/
    ├── homekit-wol.init
    └── config.example.yaml
```

## 准备配置

从 `src/go/` 执行以下复制命令（仅首次，已有实际配置时不要覆盖）：

```sh
cp -n deploy/openwrt/wol/config.example.yaml deploy/openwrt/wol/config.yaml
cp -n deploy/openwrt/temperature/config.example.yaml deploy/openwrt/temperature/config.yaml
chmod 600 deploy/openwrt/wol/config.yaml deploy/openwrt/temperature/config.yaml
```

填写目标 MAC、子网广播、HomeKit 发布接口和监听地址。两个服务使用不同端口、独立配对目录。温度路径通过 `/usr/bin/homekit-temperature -list-sensors` 确认；BE3600 示例为 `thermal_zone0`（`tsens_tz_sensor11`），不代表某个 CPU 核心。Apple Home 需能访问所选 LAN 接口的 mDNS UDP 5353 和两个 TCP 端口。

WOL 配置中的 `options` 可选，不配置时仅发送唤醒包。PVE 模式配置 `type: pve`、宿主机固定 IPv4 `host`、完整 `token`。节点名自动发现；无需证书文件，HTTPS 不核验服务器身份。详见 [Go 服务说明](../../README.md#pve-电源控制)。

## PVE Token

使用 SSH 登录 PVE，先确定实际节点名，再创建专用用户与角色：

```sh
pveum user add homekit@pve --comment 'HomeKit host power control'
pveum role add HomeKitPower --privs 'Sys.Audit Sys.PowerMgmt'
pveum acl modify /nodes/<节点名> --users homekit@pve --roles HomeKitPower --propagate 0
pveum user token add homekit@pve power --privsep 1 --output-format json
pveum acl modify /nodes/<节点名> --tokens 'homekit@pve!power' --roles HomeKitPower --propagate 0
```

将创建时仅返回一次的密钥填入被忽略的配置：`token: "homekit@pve!power=<密钥>"`。创建命令仅用于首次配置；已有用户、角色或 Token 时检查并复用，不重复执行。不要把密钥复制到文档或日志。

## 构建与首次安装

从 `src/go/` 执行，构建脚本会自动下载并应用 HAP 小补丁，完整源码不进入仓库。先确认实际配置存在并已填写；仅上传明确列出的文件，两个配置使用不同临时名称：

```sh
set -eu
./scripts/build-openwrt.sh
test -f deploy/openwrt/wol/config.yaml && test -f deploy/openwrt/temperature/config.yaml
ssh be3600 'uname -m; ip -4 addr show'
scp -O bin/homekit-wol-linux-arm64 bin/homekit-temperature-linux-arm64 deploy/openwrt/wol/homekit-wol.init deploy/openwrt/temperature/homekit-temperature.init be3600:/tmp/
scp -O deploy/openwrt/wol/config.yaml be3600:/tmp/homekit-wol.config.yaml
scp -O deploy/openwrt/temperature/config.yaml be3600:/tmp/homekit-temperature.config.yaml
```

确认架构为 `aarch64`，再在路由器执行（首次安装；已有部署参照升级流程）：

```sh
mkdir -p /etc/homekit-wol /etc/homekit-temperature
chmod 700 /etc/homekit-wol /etc/homekit-temperature
cp /tmp/homekit-wol-linux-arm64 /usr/bin/homekit-wol
cp /tmp/homekit-temperature-linux-arm64 /usr/bin/homekit-temperature
cp /tmp/homekit-wol.init /etc/init.d/homekit-wol
cp /tmp/homekit-temperature.init /etc/init.d/homekit-temperature
cp /tmp/homekit-wol.config.yaml /etc/homekit-wol/config.yaml
cp /tmp/homekit-temperature.config.yaml /etc/homekit-temperature/config.yaml
chmod 755 /usr/bin/homekit-wol /usr/bin/homekit-temperature /etc/init.d/homekit-wol /etc/init.d/homekit-temperature
chmod 600 /etc/homekit-wol/config.yaml /etc/homekit-temperature/config.yaml
rm /tmp/homekit-wol.config.yaml /tmp/homekit-temperature.config.yaml
/etc/init.d/homekit-wol enable
/etc/init.d/homekit-temperature enable
/etc/init.d/homekit-wol start
/etc/init.d/homekit-temperature start
logread -e homekit
```

在 Apple Home 分别添加两个服务；配置与 `db/` 存于持久 overlay，不使用 `/tmp`。实际配置不存在时程序会报错，不会使用示例。

## 升级、回滚与管理

升级前备份 `/usr/bin/homekit-wol`、`/etc/init.d/homekit-wol`、`/etc/homekit-wol/`，备份目录权限 `0700`，复制配置保留 `0600`。温度服务升级时同样处理。

仅更新二进制时显式上传对应二进制到 `/tmp/`，然后 stop、替换 `/usr/bin/` 文件、`chmod 755`、start，保留配置与 `db/`。更新配置时使用对应的独立临时名称，替换 `/etc/homekit-*/config.yaml`、`chmod 600`、删除临时配置，再 reload。失败时 stop、恢复备份文件、start。不要覆盖另一个服务的配置或删除配对目录。

两个服务支持 start、stop、restart、reload、enable、disable。procd 转发日志，异常退出 5 秒后重试，一小时内五次快速退出后停止重试。修复配置后手动 restart。路由器可能因旧连接的 TCP TIME_WAIT 暂时无法重新绑定端口；程序对此等待重试最多 65 秒，退出信号可取消等待。进程运行仍需配合检查监听端口，不能单凭 procd 状态判定就绪。

卸载时 stop、disable，移除二进制与 init 脚本，默认保留配置及配对目录。sysupgrade 前备份 `/etc/homekit-*/` 并检查固件保留规则；二进制可能需要重装。

## 实机验收

检查 procd 状态、监听端口、日志、温度值，以及 PVE API 节点发现和状态读取。确认 BIOS/网卡 WOL、来宾正常关机与自启动，再执行关机→连续离线→WOL→API 恢复闭环，核对来宾恢复。

当前实验环境宿主机 LAN 地址为 `192.168.8.187`：使用 `ssh -o HostName=192.168.8.187 pve`，沿用已有密钥。原 `ssh pve` 的另一个地址可能依赖宿主机内的 OpenWrt 虚拟机，关机验收使用宿主机 LAN 和物理路由器，避免恢复路径随来宾停机中断。不修改个人 SSH 配置，也不重启物理路由器。

用户在家庭 App 验证开关机和状态通知；另外检查 PVE 网页手动关机后的同步。部署成功不等同于 Apple Home 验收完成。唤醒失败明确报告结果，由现场恢复主机。

## 实机记录（2026-10-08）

- 专用权限分离 Token 可自动发现节点 `pve` 并读取状态，正常关机接口实测可用。
- 修正版通过关机→离线→WOL→API 恢复闭环：关机约 55 秒、唤醒约 45 秒；`openwrt`、`ubuntu` 均恢复运行，其他原先停止的来宾保持停止。
- `/etc/homekit-wol/db/` 内所有文件与部署前备份逐一比对一致，包含配对文件、密钥、UUID 和配置版本；温度服务继续运行。
- 已建立客户端连接时，重启后的新进程约 67 秒恢复监听，PID 保持不变，未触发 procd respawn；可取消、限时的监听重试避免 TIME_WAIT 导致崩溃循环。
- 本地及路由器两份实际配置权限均为 `0600`，Token 未出现在任何待提交文件中。
- 备份位于路由器 `/root/homekit-backup-20261008-pve-power/`。Apple Home 实际点击及客户端通知呈现仍需用户手动验收；自动测试覆盖 HAP 特征读写与检测状态通知。
