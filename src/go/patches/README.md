# 依赖补丁维护

[Go 总览](../README.md)

仓库只维护补丁、准备脚本和模块元数据，完整依赖生成到被 Git 忽略的 `third_party/`。新克隆或补丁更新后先准备依赖；OpenWrt 构建脚本自动执行准备。

## 共用规则

需要 Go 和 `patch`。准备脚本从经 Go 校验的固定版本模块缓存复制源码到临时目录，只修改副本并保留上游许可证，不修改共享模块缓存或全局 Go 配置。

应用补丁成功后才替换生成目录；下载或补丁失败时退出并保留原目录，不回退到未经修改的上游版本。版本、补丁和脚本校验值未变时复用生成目录。不要手动编辑生成源码，`go.mod` 的本地 replacement 依赖这些目录。

以下命令均从 `src/go/` 执行。两份依赖准备完成后，统一运行 `go test -race ./...`、`go vet ./...` 和 `./scripts/build-openwrt.sh`。

## HAP

### 版本与补丁目的

固定 `github.com/brutella/hap v0.0.35`，通过 `hap-v0.0.35.patch` 生成 `third_party/hap/`，保留 Apache-2.0 许可证与版权声明。

- `characteristic.C.RemoteWriteFunc` 接受远程命令但不改变缓存值，也处理同值写入；本地 `SetValue` 发布检测状态，普通配件不受影响。应用回归测试位于 `internal/wol/`。
- TCP 地址占用时可取消地重试约 65 秒，处理 OpenWrt 上的 TIME_WAIT；其他错误立即返回，不共享端口。补充测试随补丁生成。

### 准备与检查

```sh
./scripts/prepare-hap.sh
./scripts/test-prepare-hap.sh
(cd third_party/hap && go test -race ./characteristic && go test -race -run '^TestListenTCP' .)
```

### 升级上游

1. 同步更新 `go.mod` 的 HAP 版本、准备脚本的 `hap_version` 和补丁文件名。
2. 对照新版本重新生成补丁，确认写入钩子与监听重试仍需要且兼容。
3. 更新 `go.sum`，删除生成目录后重新准备，运行本节与共用检查。
4. 只提交补丁、脚本、说明及模块元数据，不提交生成源码。

## Android TV Remote v2

### 版本与补丁目的

固定 `github.com/jkiddo/atvremote v0.0.0-20260806215925-ab5a73b3c68d`，通过 `atvremote.patch` 生成 `third_party/atvremote/`，保留上游许可证。

补丁公开当前连接协商的 features 及输入字段计数器是否就绪，供 CLI 检查功能支持与等待 IME，不记录配对码或文本。

### 准备与检查

```sh
./scripts/prepare-atvremote.sh
./scripts/test-prepare-atvremote.sh
```

准备脚本测试覆盖干净生成、复用、补丁更新、失败保留和模块缓存未被修改。

### 升级上游

1. 同步更新 `go.mod` 的版本与准备脚本的 `atvremote_version`。
2. 对照新版本重新生成 `atvremote.patch`，核对功能协商与 IME 就绪接口。
3. 更新 `go.sum`，删除生成目录后重新准备，运行本节与共用检查。
4. 只提交补丁、脚本、说明及模块元数据，不提交生成源码。
