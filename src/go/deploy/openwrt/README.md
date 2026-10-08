# OpenWrt 部署

[Go 总览](../../README.md) · [HomeKit 配置](../../docs/homekit.md)

使用独立二进制与 procd 服务部署，可按需安装一个或多个应用。当前构建脚本输出 Linux ARM64 程序；先用 `uname -m` 确认路由器为 `aarch64`，其他架构需自行调整构建参数。温度服务另需可读的 Linux thermal sysfs。

| 应用 | 配置准备与验收 | 示例 TCP 端口 | 持久目录 |
| --- | --- | --- | --- |
| WOL / PVE | [WOL 部署](wol/README.md) | 32042 | `/etc/homekit-wol/` |
| 温度 | [温度部署](temperature/README.md) | 32043 | `/etc/homekit-temperature/` |
| Chromecast | [Chromecast 部署](chromecast/README.md) | 32044 | `/etc/homekit-chromecast/` |

## 构建与准备

以下本地命令从 `src/go/` 执行。将 `ROUTER_HOST` 替换为实际路由器地址，设置 `app` 为 `wol`、`temperature` 或 `chromecast`。后续本地步骤在同一 shell 中执行，每次安装一个应用：

```sh
set -eu
router='root@ROUTER_HOST'
app='wol'
service="homekit-$app"
./scripts/build-openwrt.sh
ssh "$router" 'uname -m; ip -4 addr show'
cp -n "deploy/openwrt/$app/config.example.yaml" "deploy/openwrt/$app/config.yaml"
chmod 600 "deploy/openwrt/$app/config.yaml"
```

构建脚本自动准备两份补丁依赖。按上表对应指南填写实际配置后再上传，示例中的目标地址、接口和温度路径不能直接用于实际部署。多个服务须使用不同端口、独立配对目录；Apple Home 客户端需能访问路由器 LAN 的 mDNS UDP 5353 和相应 TCP 端口。

## 首次安装

仅用于尚未部署的应用，已有服务使用下文升级流程。本地上传选定应用的三份文件，配置的临时名称按应用区分：

```sh
test -f "deploy/openwrt/$app/config.yaml"
scp -O "bin/$service-linux-arm64" "$router:/tmp/$service.new"
scp -O "deploy/openwrt/$app/$service.init" "$router:/tmp/$service.init"
scp -O "deploy/openwrt/$app/config.yaml" "$router:/tmp/$service.config.yaml"
ssh "$router"
```

在路由器 shell 中重新设置相同的 `app`，然后安装：

```sh
set -eu
app='wol'
service="homekit-$app"
mkdir -p "/etc/$service"
chmod 700 "/etc/$service"
cp "/tmp/$service.new" "/usr/bin/$service"
cp "/tmp/$service.init" "/etc/init.d/$service"
cp "/tmp/$service.config.yaml" "/etc/$service/config.yaml"
chmod 755 "/usr/bin/$service" "/etc/init.d/$service"
chmod 600 "/etc/$service/config.yaml"
rm "/tmp/$service.new" "/tmp/$service.init" "/tmp/$service.config.yaml"
```

启动前完成应用指南中的必要步骤：温度服务确认传感器路径，Chromecast 完成 Remote v2 配对。然后在同一路由器 shell 中执行：

```sh
"/etc/init.d/$service" enable
"/etc/init.d/$service" start
logread -e "$service"
```

在 Apple Home 中使用配置 PIN 添加服务。配置、配对数据库与凭据存于持久目录，不放在 `/tmp`。配置缺失时程序报错，不会使用示例。

## 管理与故障排查

各服务支持 `start`、`stop`、`restart`、`reload`、`enable`、`disable`。使用 `/etc/init.d/homekit-<应用> <操作>` 管理；修改配置后可 `reload`。

日志由 procd 转发，通过 `logread -e homekit-<应用>` 查看。异常退出 5 秒后重试，一小时内五次快速退出后停止重试；修复配置后手动 `restart`。旧连接可能使 TCP 端口暂时无法重新绑定，程序对此可取消地等待重试最多约 65 秒。检查 procd 状态之外，还应核对监听端口、日志及应用实际响应。

## 升级、回滚与卸载

升级前备份对应二进制 `/usr/bin/homekit-<应用>`、init 脚本和整个 `/etc/homekit-<应用>/`。备份目录使用 `0700` 权限，配置与私钥保留 `0600`。

1. 在本地构建并仅上传所需二进制到 `/tmp/homekit-<应用>.new`。
2. 停止对应服务，替换 `/usr/bin/` 下的二进制并设置 `0755`，再启动。
3. 如需更新 init 脚本或配置，分别上传并替换对应文件，恢复权限，删除临时配置后重启。
4. 按应用指南验收；失败时停止服务、恢复备份文件并启动。

保留配对数据库与 Chromecast 凭据，避免覆盖其他应用的文件。卸载时先 `stop`、`disable`，再移除二进制和 init 脚本，默认保留配置与配对数据。

OpenWrt 固件升级前备份相关持久目录并检查 sysupgrade 保留项；固件升级后可能需要重装二进制和 init 脚本。
