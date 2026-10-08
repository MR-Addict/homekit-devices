# 网络唤醒（wol）

通过「家庭」App 中的开关向局域网发送 Wake-on-LAN Magic Packet，唤醒指定电脑。开关触发 3 秒后自动复位，方便重复使用。

目标电脑需在 BIOS 与网卡设置中开启 Wake-on-LAN，并与 ESP32 处于同一局域网。

固件会广播 `HS-WOL-xxxx` BLE 配网服务。操作方法见 [BLE 配网页面](../../web/ble-provision/README.md)。

## 目标设备配置

编辑 `wol.ino`，每台电脑对应一个配件：

```cpp
new SpanAccessory();
new Service::AccessoryInformation();
new Characteristic::Identify();
new HSWOL("MAC地址");
```

修改或删除 `new HSWOL("...")` 中的 MAC 地址即可。

## 烧录与使用

按 [HomeSpan 共用指南](../README.md) 安装依赖、选择分区、烧录及配对。
