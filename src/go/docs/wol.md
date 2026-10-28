# WOL 与 PVE 电源控制

[Go 总览](../README.md) · [HomeKit 配置](homekit.md) · [OpenWrt 部署](../deploy/openwrt/wol/README.md)

`homekit-wol` 将多个目标暴露为 HomeKit 桥接配件。普通设备打开开关时发送 Magic Packet，两秒后复位；配置 PVE 后，开关显示检测到的电源状态，支持 WOL 开机与正常关机。

## 本地运行与配置

先按 Go 总览准备依赖。从 `src/go/` 执行：

```sh
cp -n deploy/openwrt/wol/config.example.yaml deploy/openwrt/wol/config.yaml
# 按本地环境修改 storage_path、listen_address、interfaces 和目标设备。
go run ./cmd/homekit-wol -config deploy/openwrt/wol/config.yaml
```

至少配置一个 `devices` 条目。目标电脑需启用 BIOS 和网卡的 WOL。`wol.broadcast_ip` 默认 `255.255.255.255`，`wol.port` 默认 `9`；设备可单独覆盖 `broadcast_ip` 和 `port`，建议使用目标 LAN 的子网广播地址。

使用默认有限广播地址时，还会尝试活动 IPv4 接口的广播地址。MAC 必须为标准六字节地址，名称和 MAC 不可重复。

旧版单设备 `device:` 配置需改为 `devices:`；升级为桥接可能需要重新配对。已有多设备部署沿用原配置和配对目录，并保持工作目录或设置绝对存储路径。启用 PVE 不改变配件身份和配对数据。

## PVE 配置与 Token

在目标设备条目下添加：

```yaml
options:
  type: pve
  host: "192.0.2.10"
  token: "<用户>@pve!<令牌名>=<密钥>"
```

`host` 必须为宿主机固定 IPv4 地址，使用 HTTPS 8006。当前实现跳过服务器证书验证，Token 负责认证。程序通过 `/api2/json/nodes` 自动发现唯一节点并缓存，多节点或发现失败会报错；设备 `name` 仅用于显示。

在 PVE 上确认实际节点名，然后首次创建专用用户、角色和权限分离 Token。下列 `NODE_NAME` 需替换，已有用户、角色或 Token 时检查并复用：

```sh
pve_node='NODE_NAME'
pveum user add homekit@pve --comment 'HomeKit host power control'
pveum role add HomeKitPower --privs 'Sys.Audit Sys.PowerMgmt'
pveum acl modify "/nodes/$pve_node" --users homekit@pve --roles HomeKitPower --propagate 0
pveum user token add homekit@pve power --privsep 1 --output-format json
pveum acl modify "/nodes/$pve_node" --tokens 'homekit@pve!power' --roles HomeKitPower --propagate 0
```

用户与 Token 均需目标 `/nodes/<节点名>` 上的 `Sys.Audit`、`Sys.PowerMgmt`，无需 VM 管理权限。将创建时仅返回一次的密钥填写为 `token: "homekit@pve!power=<密钥>"`，只保存在被忽略的实际配置中，设置 `chmod 600`，不要提交或输出到日志。

## PVE 状态与故障

每 5 秒查询状态，单次请求超时 3 秒。API 成功表示运行；API 返回错误或 ICMP 可达表示在线但管理异常，此时 HomeKit 读取返回通信错误，日志记录故障。API 无响应且连续三轮 ICMP 无响应才推定离线；探测自身失败不判定关机。断网可能被推定为关机。

首次状态未知时读取不可用，但允许发送开机命令。点击开关仅发起命令，开机后 API 恢复才显示开启，关机后检测离线才显示关闭。关机采用 PVE 正常宿主机关机流程，来宾处理遵循已有配置。

状态转换最多等待 5 分钟，同方向重复请求合并，转换期间拒绝反向请求；超时记录错误并继续检测，不自动重复关机。Apple Home 对通信错误与转换过程的显示取决于客户端。

## 通用 Linux Agent

Linux 目标添加以下配置；桥接运行在路由器等常开设备，Agent 运行在目标主机：

```yaml
options:
  type: agent
  host: "192.0.2.10"
  token: "<与 Agent 相同的随机 Token>"
  # port: 32045  # 可省略；自定义端口时覆盖
```

Agent 安装与 HTTP 协议见 [Linux power-agent](../deploy/linux/power-agent/README.md)。当前支持 systemd Linux，接口及电源实现分离，未来可增加 Windows 后端。Agent 不参与 HomeKit 配对；开机使用已配置 MAC 的 WOL，关机使用 HTTP API，无需在桥接保存 SSH 凭据。

Agent 使用 `/api/status`、`/api/shutdown`，HTTP Bearer Token 认证。HTTP 明文仅用于可信内网；配置保持 0600，不记录或提交 Token。

状态检测、请求超时、连续三轮离线确认与五分钟转换期限沿用上述电源控制规则。`shutting_down` 在离线前仍视为开启；API 认证失败或进程故障但 ICMP 可达时报告通信故障。断网无法与关机完全区分。

可显式选择实机验收（会真实操作目标，普通 `go test` 不连接设备）：先用 `GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go test -c -o bin/wol-live-linux-arm64 ./internal/wol` 构建，在路由器设置 `HOMEKIT_POWER_CONFIG=/etc/homekit-wol/config.yaml`，执行测试二进制 `-test.run '^TestLivePower$' -test.v` 只读检查；增加 `HOMEKIT_POWER_ACTION=off` 或 `on` 分别验证正常关机、WOL 开机及 HAP 状态转换。当前验收配置须只有一个电源目标。
