# Linux power-agent

Agent 提供通用 HTTP 电源接口，与 HomeKit 独立。当前系统后端支持 systemd Linux，需要 systemd-logind、polkit，以及支持 `LoadCredential` 的 systemd（247+）。未来可增加 Windows 或其他 Linux 初始化系统后端，沿用协议。

## 构建与安装

从 `src/go/` 构建（NUC 使用 amd64，ARM64 主机改为 arm64）：

```sh
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o bin/power-agent-linux-amd64 ./cmd/power-agent
```

在目标主机创建无登录权限的系统用户 `power-agent`，安装二进制到 `/usr/local/bin/power-agent`（0755），本目录服务文件到 `/etc/systemd/system/power-agent.service`，规则文件到 `/etc/polkit-1/rules.d/50-power-agent.rules`（root 所有、0644）。首次配置：

```sh
sudo useradd --system --no-create-home --shell /usr/sbin/nologin power-agent
sudo install -d -m 700 /etc/power-agent
sudo sh -c 'umask 077; printf "token: \"%s\"\n" "$(openssl rand -hex 32)" > /etc/power-agent/config.yaml'
sudo systemctl daemon-reload
sudo systemctl enable --now power-agent
```

已有用户或配置时复用，不重新生成 Token。配置示例见本目录 `config.example.yaml`；省略 `listen_address` 默认 `:32045`，可显式覆盖为 `IP:端口`。root 所有的配置保持 0600；systemd 通过 LoadCredential 将其安全提供给专用用户。共享 Token 填入桥接的私有配置，不提交版本库，不输出到日志。

Agent 使用非 root 用户，通过 logind 的 `PowerOff(false)` 正常关机，只授权 power-off 和 power-off-multiple-sessions，不授权忽略 inhibitors。关机被阻止或权限失败时返回 503。规则自动由 polkit 加载。

## HTTP 协议

所有接口要求 `Authorization: Bearer <token>`，缺失或错误返回 401。只在可信内网提供服务；HTTP 明文传输 Token，不转发到公网。接口没有版本前缀。

- `GET /api/status` → 200 JSON：`{"hostname":"pc","boot_id":"...","state":"running"}`。已接受关机时 state 为 `shutting_down`，此时机器仍在线。
- `POST /api/shutdown` → 系统接受正常关机后返回 202。并发、重复请求在进程生命周期内合并；失败返回 503，仍可重试。进程收到系统终止信号时优雅退出，实际关机可能先于客户端收到响应。
- 方法不符返回 405，未知路径返回 404；不提供任意命令执行接口。

GET 可用于认证、网络、重启后的启动 ID 验证；POST 会真实关闭主机。普通自动测试使用模拟系统后端，不关闭机器。

## 运维

```sh
sudo systemctl status power-agent
sudo journalctl -u power-agent
```

升级前备份二进制、配置、unit 和 polkit 规则；替换二进制后重启服务。失败时恢复备份；保留原 Token。卸载前停止、禁用服务，再移除二进制、unit 和规则，默认保留私有配置。

桥接通过配置的 IP 连接主机，地址变化后需更新 `options.host`；可按需配置固定 IP 或 DHCP 保留。有线网卡与 BIOS 需开启 WOL。NetworkManager 可用 `nmcli connection modify '<有线连接>' 802-3-ethernet.wake-on-lan magic` 持久开启，`ethtool <网卡>` 验证当前为 `Wake-on: g`。WOL 开关与 Agent 分别验收。
