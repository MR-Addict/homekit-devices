# HAP 小补丁

基于 `github.com/brutella/hap v0.0.35`。仓库只维护 `hap-v0.0.35.patch` 与准备脚本，不跟踪完整依赖源码。

补丁保留两项行为：

- `characteristic.C.RemoteWriteFunc` 接受远程命令但不改变缓存值，也处理同值写入；本地 `SetValue` 继续发布检测状态，普通配件不受影响。回归测试位于 `internal/wol/`。
- TCP 绑定遇到地址占用时限时重试约 65 秒，等待 OpenWrt 上观察到的 TIME_WAIT 消退；等待可取消，其他错误立即返回，不共享端口。补充测试 `listen_tcp_test.go` 随补丁生成。

## 准备与检查

从 `src/go/` 执行：

```sh
./scripts/prepare-hap.sh
./scripts/test-prepare-hap.sh
go test -race ./...
go vet ./...
(cd third_party/hap && go test -race ./characteristic && go test -race -run '^TestListenTCP' .)
./scripts/build-openwrt.sh
```

首次开发或补丁更新后运行准备脚本，随后正常使用 `go test`、`go run`、`go build`。OpenWrt 构建脚本会自动准备。需要 Go 和 `patch` 命令；脚本不修改全局 Go 配置。

脚本按固定版本从 Go 模块缓存复制源码到临时目录，保留上游 LICENSE，只修改副本。成功应用补丁后才替换 `third_party/hap/`；下载或补丁失败时明确退出并保留现有目录，不回退到未经修改的上游版本。版本、补丁及脚本校验值未变时复用生成目录；不要手动编辑生成源码。HAP 仍由 Go 正常下载并校验。

`third_party/hap/` 和临时准备目录均被 Git 忽略。`go.mod` 的本地 replacement 保持不变，因此新克隆必须先准备依赖；无需独立 fork 仓库或为每次 Go 命令增加特殊参数。

## 升级上游

1. 同时更新 `go.mod` 的 HAP 版本、准备脚本的 `hap_version` 和对应补丁文件名。
2. 对照新版本重新生成补丁，确认写入钩子与取消监听等待的行为仍需要并兼容；禁止对共享模块缓存应用补丁。
3. 更新 `go.sum`，删除生成目录并重新准备，运行上面的检查和 ARM64 构建。
4. 仓库只提交补丁、说明、脚本及模块元数据，不提交生成目录。

补丁源自 HAP 的 Apache-2.0 许可代码，生成目录保留原 LICENSE 和版权声明。
