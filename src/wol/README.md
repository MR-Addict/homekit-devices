# 网络唤醒（wol）

通过「家庭」App 中的开关向局域网发送 Wake-on-LAN Magic Packet，唤醒指定电脑。开关触发 3 秒后自动复位，方便重复使用。

目标电脑需在 BIOS 与网卡设置中开启 Wake-on-LAN，并与 ESP32 处于同一局域网。

固件会广播 `HS-WOL-xxxx` BLE 配网服务。使用项目的通用配网页面连接设备，首次验证 OTA 密码 `ota` 后设置 Wi‑Fi；验证成功的密码会保存在当前浏览器，后续连接时自动验证。页面的本地运行与 GitHub Pages 地址见项目总 README。

## 添加要唤醒的电脑

编辑 `wol.ino`，每台电脑对应一个配件：

```cpp
new SpanAccessory();
new Service::AccessoryInformation();
new Characteristic::Identify();
new HSWOL("MAC地址");
```

修改或删除 `new HSWOL("...")` 中的 MAC 地址即可。

## 文件

- `wol.ino`：入口，声明要唤醒的设备
- `wol.h`：`HSWOL` 开关服务（`Service::Switch`），发送唤醒包
- `../common/ble_provision.h`：与其他设备共用的 BLE 配网服务
