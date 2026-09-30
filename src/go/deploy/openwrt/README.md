# BE3600 OpenWrt 部署

目标为 `ssh be3600`：GL.iNet GL-BE3600 / IPQ5332，OpenWrt 23.05-SNAPSHOT，Linux 5.4.213，aarch64。这里保存实际生产配置。

```text
openwrt/
├── README.md
├── temperature/
│   ├── homekit-temperature.init
│   └── temperature.config.yaml
└── wol/
    ├── homekit-wol.init
    └── wol.config.yaml
```

## 当前配置

| 应用 | 配置 | 端口 | 配对数据 |
| --- | --- | --- | --- |
| WOL | PVE主机，MAC `E0:51:D8:11:3D:CE`，广播 `192.168.8.255:9` | TCP 32042 | `/etc/homekit-wol/db` |
| 温度 | `/sys/class/thermal/thermal_zone0/temp`，每 30 秒 | TCP 32043 | `/etc/homekit-temperature/db` |

WOL 桥接发布名称为 `Wake on LAN`，配对后可在家庭 App 中改为“网络唤醒”；内部开关仍叫“PVE主机”，可通过 `devices` 列表添加更多设备。两个 HomeKit 服务在 `br-lan`（192.168.8.1/24）发布，分别配对，默认 PIN `001-02-003`。WOL 目标 PVE主机位于 OpenWrt 下的 `192.168.8.0/24`，广播经 `br-lan` 发出。温度设备发布名称使用 `Router Temperature`，避免中文 HTTP Host 的兼容问题；配对后可以在家庭 App 中改为“路由器温度”。温度路径对应 `tsens_tz_sensor11`；固件还暴露 sensor12–15，未标明传感器在芯片上的具体位置，不能断言这是某个 CPU 核心温度。读数按千分之一摄氏度转换。

服务面向 OpenWrt 下的 LAN 设备。Apple Home 所在设备需要能访问 `br-lan` 的 mDNS UDP 5353 和两个 TCP 端口。

## 构建与首次安装

从 `src/go/` 执行：

```sh
./scripts/build-openwrt.sh
ssh be3600 'uname -m; ip -4 addr show'
scp -O bin/*-linux-arm64 deploy/openwrt/wol/* deploy/openwrt/temperature/* be3600:/tmp/
```

确认 `uname -m` 为 `aarch64`。在路由器执行（首次安装；已有部署使用后面的升级流程）：

```sh
mkdir -p /etc/homekit-wol /etc/homekit-temperature
chmod 700 /etc/homekit-wol /etc/homekit-temperature
cp /tmp/homekit-wol-linux-arm64 /usr/bin/homekit-wol
cp /tmp/homekit-temperature-linux-arm64 /usr/bin/homekit-temperature
cp /tmp/homekit-wol.init /etc/init.d/homekit-wol
cp /tmp/homekit-temperature.init /etc/init.d/homekit-temperature
cp /tmp/wol.config.yaml /etc/homekit-wol/config.yaml
cp /tmp/temperature.config.yaml /etc/homekit-temperature/config.yaml
chmod 755 /usr/bin/homekit-wol /usr/bin/homekit-temperature
chmod 755 /etc/init.d/homekit-wol /etc/init.d/homekit-temperature
chmod 600 /etc/homekit-wol/config.yaml /etc/homekit-temperature/config.yaml
/usr/bin/homekit-temperature -list-sensors
/etc/init.d/homekit-wol enable
/etc/init.d/homekit-temperature enable
/etc/init.d/homekit-wol start
/etc/init.d/homekit-temperature start
logread -e homekit
```

在 Apple Home 分别添加“Wake on LAN”桥接和“Router Temperature”传感器。两个配置目录和配对数据位于持久 overlay，不使用重启后消失的 `/tmp`。

## 管理与升级

两个服务均支持 `start`、`stop`、`restart`、`reload`、`enable`、`disable`。修改配置后执行对应服务的 reload。procd 转发系统日志，异常退出 5 秒后重试，一小时内连续五次快速退出后停止重试；修复配置后手动 restart。

更新二进制时重新构建、上传到 `/tmp/`，然后分别 stop、替换 `/usr/bin/` 中的对应二进制、`chmod 755`、start。保持配置与 `db/` 不变。更新配置时仅替换对应 `/etc/homekit-*/config.yaml` 再 reload。

卸载时 stop、disable，再移除 `/usr/bin/` 二进制和 `/etc/init.d/` 脚本；默认保留配置及配对目录。固件 sysupgrade 前备份两个 `/etc/homekit-*/` 目录，并核对固件保留规则，手动安装的二进制可能需要重装。

## 验证边界

部署后检查 procd 状态、监听端口、日志、温度原始值及服务重启。Apple Home 配对和实际 PVE 唤醒需要在家庭 App 操作验证；不通过重启路由器来影响其他网络服务。

已通过 `ssh be3600` 安装两个服务并启用开机自启，确认进程运行、TCP 端口监听和传感器发现。配对与实际唤醒仍待 Apple Home 验证。
