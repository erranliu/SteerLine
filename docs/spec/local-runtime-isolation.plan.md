# Plan：本机安装与调试路径隔离

任务 ID：83；任务名称：本机安装与调试路径隔离。唯一执行依据：[Spec](local-runtime-isolation.spec.md)。无未完成硬前置，一个独立实现 PR 交付。

## 实施顺序

1. 核对现有 InstallSelf、InstallLaunchers、HomeDir、build.ps1、console 启服和项目 re-exec，列出实际 exe/home/config 的传递位置；沿当前 app/host/product 接口实现，不恢复已删除的 facade。
2. 提供 scripts/debug-runtime.ps1，显式接收调试 Home、Config、日常 InstalledHome 及服务/控制台/CLI 操作，定位本工作树构建产物。规范化路径并拒绝重叠、越界和缺失，环境只作用于本次进程；本脚本为规划入口，实施时交付。
3. 修正调试 console 启服及子进程的 exe/home 传递，保持安装入口使用安装 exe；补充最小构建 commit/dirty 信息和运行诊断。复用实例核验与仓库锁，不增加 channel 或选择记录。
4. 在现有 E2E testenv/driver/native 适配层增加临时安装目录与源码调试入口能力，登记 runtime.installed-debug-isolation。每个项目显式选择实际 exe，复用受控后端及原断言；不另建任务状态机或绕过产品命令。
5. 完成 LI1–LI4 的同一次 Windows 现场验证及简短维护说明，向 #42 交接安装/调试命令、实际路径与证据。失败、清理失败或身份不明不能通过。

## 测试计划

| 级别/组或场景 ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| L1 固定组 + 受影响入口定向用例（LI2/LI3） | Windows、无 live 开关 | go run ./cmd/test-feedback -level=L1；实施时补充受影响 app/host/console 包的精确 -run 命令 | 路径接线完成后一次 | 固定组及实际受影响用例非零命中通过，实例核验/仓库锁未削弱 | docs/validation/local-runtime-isolation/l1/，登记实际空 -out 目录 |
| L3 runtime / runtime.installed-debug-isolation（新增，LI1–LI4） | Windows controlled、两独立 clone/home、临时安装及工作树调试产物 | go run ./cmd/test-feedback -level=L3 -suite=runtime -env=controlled '-scenario=runtime.installed-debug-isolation' -out docs/validation/local-runtime-isolation/coexistence | 新场景登记及入口完成后 | 同时运行、调试重建/停启、子进程身份和拒绝样例通过；安装摘要/实例/任务不变；本次资源清理成功 | coexistence/summary.json、scenarios/runtime.installed-debug-isolation/ |
| 编译检查 | Linux 目标、无原生进程执行 | $env:GOOS='linux'; $env:GOARCH='amd64'; go build -o <本次临时目录>/steerline ./cmd/steerline；执行后恢复原环境 | Go 接线修改后 | 当前入口及依赖编译通过；不记为 Linux 安装验收 | docs/validation/local-runtime-isolation/build-linux.log |

新增 runtime 组、场景和产物/入口能力须同步 tests/e2e/suites.json 与 README。共存场景用同一构建的两个路径即可验证隔离，调试重建后重新核对实际产物；不为“不同版本”另造协议。现有 E2E 默认构建不能替代实际安装/调试路径验证。

不选择完整 L2、live delivery/recovery/candidates 或 Full：本任务改变入口与隔离，不改变业务交付/恢复语义；受控共存场景覆盖新增风险。若实际修改到业务恢复或执行适配器，再按影响选择已有 recovery/live 场景并登记原因。每次使用空证据目录，不覆盖失败产物。纯规划阶段仅检查文档身份、链接和依赖，不执行上述产品测试。
