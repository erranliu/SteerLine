# #67 主包隔离守卫与测试归位

当前工作区：`C:/Users/lmas1/.codex/worktrees/2829/SteerLine`，分支 `codex/67-core-boundary`。代码候选 `1347b38` 的规定 quick 已完整通过；其后只修正未纳入 quick 的 Linux CLI 测试读取 helper（不再构造模拟 failed 状态，而检查真实 ConnectService 的 offline/ProcessAbsent），集成包编译通过，未执行 Linux 测试体。最终 PR head 见 Git，源码清单记录最终文件摘要。

原 f775/51 文件/未通过第三闭环等文字全部归 [历史 checkpoint](history-checkpoints.md)，仅描述当时候选，不能覆盖当前结果。

## 当前所有者

- 根包直属全部 Go 文件名和源码大小写不敏感 `codex|orca` 零命中；复制的内置平台工厂、native dispatcher、配置/绑定/linked workspace 与恢复消费 helper 已删除。中心测试只使用同一中性目录与 execution/contracts 有界事实、真实中心事务和单写者。
- 原生 codec/命令/身份/版本/原始流断言归各实现包；实际默认目录、应用配置与 CLI/跨层/live 入口归 `steerlineapp`/`steerlineintegration`，不获得私有中心写接口。完整 [断言映射](test-mapping.json)、[fixture/消费者映射](fixture-mapping.json)、[原输入清单](root-test-inventory.json)。
- 原 agent 未识别的临时宿主事实通过通用 `LifecycleReceipt.OriginalHost` 深复制保存，合格 process 仍为空；实现只读检测原终端，保持原 nonce/deadline/operation，然后单独提交 agent-bind 资格。此修复保留原检测断言，不添加 root 具体实现分支或新业务状态机。
- 外层第三中性实现使用同一目录经真实 app/Service/query/answer/事务闭环，保留原输入仅一次、重建目录/来源/服务后只读对账、新人工确认、原 session/generation/process/target、原 provider merge、唯一 TaskDelivery 与仅一次资源结算。
- `coreboundary` 无视构建标签扫描所有直属 tests/文件名和完整项目 import 图，检查直接/传递与反向依赖，并采样 Windows/Linux 生产依赖。除 Git 元数据外不排除目录；顶层/嵌套 bin、额外 OS/tag 的负向样本都必须命中并拒绝。

## 最小入口与阶段证据

```powershell
go run ./cmd/test-feedback -quick -- ./internal/steerlineintegration -count=1 -run '^(TestCD67CoreBoundary|TestCD67AdditionalImplementation)$'
go list -deps ./internal/steerline
git diff --check
```

quick 同一次组合覆盖中心原 claim/delivery 拒绝及原准备未知回执/人工屏障；implementation 的 codec/raw-stream/原身份和原生命令代表；execution/contracts/query/sqlite 等规则；外层两个 CD67 parent、七个负向样本、第三实现、两条支持路径的恢复/提交屏障、remote Protocol 与真实 Claim 拒绝。每个指定包/parent/subtest 都核对 JSON run/terminal，拒绝零命中、skip 与重复包；remote 确定性原用例单独选择，不选可选 live。

本工作区 `bin/cd67/` 保存完整定向报告和诊断：中心环境 six-case 组修复后 5/5、unknown-receipt kernel 1/1、原生 OriginalHost 检测 1/1、实现 qualified process 检测 1/1、zero boundary 完整通过、neutral config/runtime/idle 代表除 missing directory 的已修复诊断外通过、CLI 旧权威与不可见控制台代表通过。初始编译/fixture/错误分类失败报告保留，没有豁免。最终规定组合 `complete=true`、`passed=true`、errors=[]，准备 0.9s、执行 205.0s，127 个 parent/subtest pass、0 fail/skip，14 个包实际完成且各一次。完整 [组合报告](validation-results.json)、[命中事件](test-hits.json)、[边界结果](boundary-checks.json)、[源码清单](source-manifest.json)。Windows `go list -deps ./internal/steerline` 通过且无实现/app/integration，`git diff --check` 通过。

首轮 quick `20261003T030238.710450500Z` complete/failed：remote 用例的 helper 未接收真实 BackendError，以及可选 P10 被选入。身份拒绝断言没有删除；修复 helper 接受该实际类型后定向通过，remote 以明确确定性原测试选择并保持 live 条件原样。修复后一次规定完整组合为 `20261003T030829.921758400Z`，候选 clean `1347b387433bc0ebc2d50da0e2db49f764a25128`。

ROOT 最终 173 个 Go 文件，完整源码包 26 个，Windows/Linux 生产依赖 232/221，0 个违规（原始 JSON 空切片为 null）。两个父入口、七个负向样本、第三实现及五个支持/remote 子入口均有 run/pass。

## 支持限制与未运行内容

未启用任何可选 live/P10/原生权限采集；这些入口保留原 opt-in、工具、版本、原问题/身份及证据条件。旧 detached survival opt-in 的失败断言保留，当前 managed worker 不新增 detached fallback。Windows/Linux 依赖图已检验，Linux 测试体未执行。remote 仍缺原生权限落实证据，真实 Claim 在准入前拒绝；协议 mock 和第三 fake 不构成权限或双机认证。

所有本次测试使用隔离 Git/SQLite/协议 mock 与临时 STEERLINE_HOME；只调用隔离测试构建的 CLI 和临时数据库，未调用真实产品业务 CLI/任务数据库，未写真实 runtime，未修改仓库外 runner 状态/技能/automation。先前默认 runtime 配置事件及恢复来源要求仅属于历史记录，不能在此任务猜测或覆盖原配置。
