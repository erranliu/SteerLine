# 全量测试与失败修复记录

开始日期：2026-10-03，完成日期：2026-10-04（Asia/Shanghai）。依据：用户要求按 `tests/e2e/README.md` 执行所有测试，修复失败并复测。基线提交：`184e1e0`。原始失败报告保留，不覆盖或改写失败为通过。

## 修复

| 问题 | 根因与修复 | 验证 |
| --- | --- | --- |
| Windows 诊断 JSON 替换拒绝访问 | PowerShell 等读者可能不共享删除权限；对短暂 sharing/access 冲突执行最多 500ms 的重试，始终保留上一份完整快照，不删除目标文件 | 新增真实 Windows 占用句柄测试，验证释放前保留旧内容、释放后更新且无临时文件残留 |
| 服务预检解码 profile 身份失败、客户端/服务摘要不一致 | 预检直接更新 Git identity，却未重新绑定已接受的实现封装；现在复用 `ResolveIdentity`。客户端与服务同时解析可用 PR 查询/关闭命令，避免服务单独将命令转换为绝对路径后改变摘要。读取原有冻结任务不要求当前 provider 可用；新工作仍由预检严格校验 | profile 客户端/预检一致性测试及原 provider 恢复/remote drift 测试 |
| 首次只读预检依赖已存在的运行目录 | 原生进程探针日志写在尚未建立的 RunDir；改在系统临时目录创建并关闭/删除，预检不创建运行状态 | profile 预检测试保留“RunDir 不存在”断言 |
| Codex 历史初始化失败缺少诊断 | 原接口只返回 bool，丢失底层原因；改为保留错误和 Cause，并在探针诊断中呈现 | 新增错误传播和失败前不启动 worker 的测试 |
| 缺少 native authority 的负例 panic | 通用测试包装函数先构造合法执行配置，掩盖被测直接入口；改为直接调用生产入口 | 原 authority 错误及无状态目录副作用断言保留 |
| profile 配置测试沿用旧契约 | 配置了不能由 Orca 执行的 host publisher；改为显式配置各 profile 的发布职责。只读 catalog 使用 query loader，并继续拒绝缺少远端 enforcement evidence 的选择。预检测试使用确定性 app-server 协议夹具及完整 close 命令 | 全部 steerlineapp 包测试 |
| 丢失终端创建回执的集成断言过时 | 只读协调可能在快照返回前已经恢复原终端；改为核对其 terminal/runtime/incarnation/worktree 完整原身份，并要求环境仍未 ready、继续显式 recheck | 原生协议集成定向测试 |
| 慢恢复用例未进入被测恢复路径 | 夹具继承了 ResumeActiveObservation=true，且阻塞注入在旧 ObserveLifecycle；改为声明对应恢复能力并在实际 Reconcile 入口阻塞 | 原写线程不阻塞、重复调用不重派、取消后后台退出断言保留；未扩大超时 |
| 真实 Orca 模型回执省略 PR URL owner | 发布脚本创建了合法 PR，但模型重写为错误 URL；明确要求在 final 和 completion relay 中原样复制完整 URL。不放宽生产 PR 验证 | L2 真实 Orca PR #47 完整验证并合并 |

第一次 L2 还发现 Orca 桌面应用未运行，已启动并确认 CLI/runtime 可达；未改产品连接规则。一次真实 local 预检历史初始化失败未稳定复现，后续真实交付通过；记录该失败并补齐诊断，不声称已经证实其具体根因。

## 执行范围

按文档运行默认确定性全量入口（`-workers=3`）、L1、L2 gated 全部 3 场景、L3 delivery/recovery 的 controlled 全部 2 场景和 live 全部 4 场景；另运行带 e2e tag 的内部环境测试，覆盖双项目隔离及部分准备清理。

文档说明 elevated sandbox 账号工具不属于 L2/L3 前置条件，因此未执行管理员初始化。默认套件保留额外原生验收开关及进程 helper 的默认 skip；没有新增 skip，也没有把这些跳过项计作真实 E2E 通过。

## 测试结果与证据

报告位于 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/`，每次运行独立目录，包含 summary、原始 Go JSON、场景状态和清理证据。

| 范围 | 命令 | 结果 / 报告目录 |
| --- | --- | --- |
| 初始 full | `go run ./cmd/test-feedback -workers=3` | 失败：`20261003T151800.579836900Z`；4 个已确认失败，core panic 导致部分用例未执行 |
| 第一次 full 复测 | 同上 | `20261003T154057.696491300Z`；发现 provider 可用性边界和慢恢复夹具两处问题，均已定向修复 |
| 最终 full | 同上 | 通过：`20261003T155406.420144000Z`；complete=true、零失败，960 条测试记录中 19 项默认 skip，执行 1530.9 秒 |
| L1 复测 | `go run ./cmd/test-feedback -level=L1` | 通过：`20261003T154631.184352500Z` |
| L2 全部 | `go run ./cmd/test-feedback -level=L2` | 通过：`20261003T153725.584438000Z`，3/3 场景，清理完成 |
| L3 controlled | `go run ./cmd/test-feedback -level=L3 '-suite=delivery,recovery' -env=controlled` | 最终复核通过：`20261003T160145.165383400Z`，2/2 场景，清理完成；先前通过报告 `20261003T153406.885826800Z` |
| L3 live | `go run ./cmd/test-feedback -level=L3 '-suite=delivery,recovery' -env=live` | 通过：`20261003T154620.546062000Z`，4/4 场景，清理完成 |
| 内部 tagged 环境 | `go run ./cmd/test-feedback -- -tags=e2e ./tests/e2e/internal/... -count=1` | 最终复核通过：`docs/validation/test-feedback/runs/20261003T160530Z/summary.json`；额外 sandbox 凭据 opt-in 测试按默认 skip |
| 定向修复回归 | authority、profile selection/preflight、原 provider 恢复、慢恢复、Orca 原终端协调、Windows 文件替换 | 通过 |
| 包回归 | `go test ./internal/steerlineapp ./internal/steerline/codexworker ./internal/testboundary -count=1` | 通过 |

L2 真实已合并 PR：[local #46](https://github.com/erranliu/SteerlineTest/pull/46)、[Orca #47](https://github.com/erranliu/SteerlineTest/pull/47)。两场景的 `scenario.json` 均确认 passed 和 cleanup_complete，原模型 usage 及 GitHub merged 状态证据在对应报告内。

最终核对：L1、full、9/9 登记 E2E 场景及 tagged 内部环境验证均通过；三个 E2E 报告都确认 cleanup_complete=true。通过 `gh pr view` 再核对失败运行的 [PR #45](https://github.com/erranliu/SteerlineTest/pull/45) 已 CLOSED，#46/#47 均 MERGED。`git diff --check` 通过。没有新增 skip、降低业务断言或扩大场景超时；改动保留在当前工作树，尚未提交。
