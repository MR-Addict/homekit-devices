# HomeKit 通用配置

[Go 总览](../README.md) · [OpenWrt 部署](../deploy/openwrt/README.md)

三个程序均通过 `-config FILE` 指定 YAML 文件，省略时读取当前工作目录的 `config.yaml`。缺少实际配置会报错，不会回退到示例。

## 共享字段

| `homekit` 字段 | 说明 |
| --- | --- |
| `name` | Apple Home 中的名称 |
| `pin` | 八位数字或 `3-2-3` 格式，不能使用 HomeKit 保留的无效 PIN |
| `storage_path` | 配对数据目录，升级时保留 |
| `listen_address` | TCP 监听地址，格式为 `host:port`，如 `:32042` |
| `interfaces` | 发布 HomeKit 服务的网络接口，如 `["br-lan"]`；本地运行需按实际接口修改 |
| `serial_number` | 配件序列号，多实例应分别设置 |
| `manufacturer`、`model`、`firmware` | 配件元数据 |

同时运行多个服务时使用不同监听端口、独立配对目录，并分别添加到 Apple Home。客户端需能访问所选 LAN 接口的 mDNS UDP 5353 和各服务 TCP 端口。

## 程序默认值与部署示例

三个程序默认 PIN 均为 `001-02-003`。以下是未显式配置时的程序默认值：

| 程序 | 名称 | 存储目录 | 监听地址 |
| --- | --- | --- | --- |
| WOL | `Wake Targets` | `./db` | 使用 HAP 默认地址 |
| 温度 | `路由器温度` | `./temperature-db` | 使用 HAP 默认地址 |
| Chromecast | `Chromecast` | `./chromecast-db` | `:32044` |

OpenWrt 示例显式设置端口 `32042`、`32043`、`32044`，并将数据存于对应的 `/etc/homekit-<应用>/db`。示例中的名称、接口和地址需按实际环境填写，不等同于程序默认值。

## 相对路径与数据保留

- WOL、温度的相对 `homekit.storage_path` 以进程工作目录为基准。
- Chromecast 的相对存储目录、证书和私钥路径以配置文件所在目录为基准。

部署建议使用绝对路径，避免工作目录变化导致配对数据被保存到新位置。配置示例复制为同目录 `config.yaml` 后再填写，实际配置被 Git 忽略；含 Token 的配置及配对私钥使用 `0600` 权限。升级和回滚保留原配置、配对目录及凭据。
