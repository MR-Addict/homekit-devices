# BLE 配网页面

为 [HomeSpan 设备](../../homespan/README.md) 设置 Wi-Fi，即使设备失去 Wi-Fi，也可从附近电脑重新配置。

## 使用

使用支持 Web Bluetooth 的桌面 Chrome 或 Edge，在 HTTPS 或 localhost 环境打开页面。iPhone / iPad Safari 不原生支持 Web Bluetooth。

打开 [在线配网页面](https://MR-Addict.github.io/homekit-devices/)，或启动下方本地开发服务器。

1. 搜索并连接设备，设备名以 `HS-Fan-`、`HS-Lamp-` 或 `HS-WOL-` 开头，后四位用于区分设备。
2. 首次连接验证 OTA 密码，默认 `ota`。
3. 填写 Wi-Fi 名称和密码并保存。新版固件验证后会回填已保存凭据，旧固件可手动填写。
4. 设备确认保存后，页面显示设置完成；设备随后重启并尝试连接 Wi-Fi。完成提示不表示已联网，输错时可重新连接并修改。

SSID 最多 32 个 UTF-8 字节，密码最多 64 个 UTF-8 字节；开放网络密码可留空。联网后按 HomeSpan 指南完成 HomeKit 配对。

## 凭据处理

验证成功的 OTA 密码保存在当前浏览器的 localStorage 中，供下次连接自动验证。Wi-Fi 凭据只在当前页面会话中回填，不保存在浏览器持久存储或上传到服务器。

BLE 服务通过应用层 OTA 密码验证操作，但当前特征值未要求 BLE 配对或加密连接。密码 `ota` 已公开于代码和文档，只能避免误操作，不能阻止附近知晓密码的人读取或修改 Wi‑Fi 凭据。

## 开发与检查

当前 CI 使用 Node.js 26 和 pnpm 12。以下命令从 `src/web/ble-provision/` 执行：

```sh
pnpm install --frozen-lockfile
pnpm dev
```

用桌面 Chrome 或 Edge 打开 Vite 显示的本地地址。测试、构建及预览：

```sh
pnpm test
pnpm build
pnpm preview
```

## 发布

推送到 `main` 或手动触发 GitHub Actions 的 `Deploy GitHub Pages` 工作流，会安装依赖并执行 `pnpm build`，将 `dist/` 发布到 `gh-pages` 分支。首次发布在仓库 Pages 设置中选择 `gh-pages` 分支作为发布源。
