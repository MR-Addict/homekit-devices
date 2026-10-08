# 温度传感器 OpenWrt 部署

[通用安装与维护](../README.md) · [应用使用与配置](../../../docs/temperature.md)

## 配置准备

按通用流程选择 `app='temperature'`，复制本目录 `config.example.yaml` 为 `config.yaml`。示例端口为 `32043`，配对目录为 `/etc/homekit-temperature/db`，LAN 接口按实际环境填写。

在通用流程安装二进制后、启动前，在路由器执行：

```sh
/usr/bin/homekit-temperature -list-sensors
```

根据输出的传感器 `type` 选择实际 `temp` 路径，并更新 `/etc/homekit-temperature/config.yaml` 的 `temperature.path`。示例 `thermal_zone0` 只是占位路径，不表示 CPU；也可先上传二进制枚举传感器，再填写本地配置。

按通用流程启动并添加到 Apple Home。更新间隔默认 `30s`，字段与读数故障行为见应用文档。

## 验收

- 传感器文件可读，家庭 App 温度与原始千分之一摄氏度读数一致。
- 温度按配置间隔更新，日志无读取或范围错误。
- 重启服务后配对保留；如需验证故障恢复，使用临时测试文件或测试配置，不修改 sysfs 传感器文件。
