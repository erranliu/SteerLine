# 运行时测试进度验收

2026-10-24；关联 [Spec](../../spec/runtime-test-progress.spec.md) / [Plan](../../plans/runtime-test-progress.plan.md)。SteerLine 初轮基线 `908c2570`，提 PR 审计时已 rebase 到主干 `65a51586`，下文首轮与审计后结果按各自报告的实际源码身份解释；Runner 公开接口来源为 `C:/Project/SteerRunner` 的 `aea8abe`。本次不修改 Runner 或安装版，不接管 Task 18，不迁移旧库。

## 实现

候选冻结 `progress_file` 初始布尔对象，严格消费 Runner 单文件快照；独立 SQLite `candidate_progress` 保存原身份、版本、最后有效映射和诊断。错误或断连保留观察，恢复同一 Run 可续读，进度不影响业务 revision/输入。主干 Git 历史已使用 schema 30，本次顺延到 31 并明确拒绝旧 schema。

清除候选 R2 客户端、收集 outbox/证据表和当前 Git 证明投影。同步保留主干的新 Git JSON 历史导出及主分支只读机制，不恢复已退役的 register/数据库历史或复验关系。执行页仅显示通过/未完成与同步状态，清除候选旧历史、日志和失败计数字段。

SteerLine Go 入口、PowerShell 通用入口及 merge-h-308-code L1–L4 共用字节锁/合并/原子替换协议。开始/重跑清零，真实完整通过才完成；未安排项不新增，partial/失败/跳过不通过。业务项目保留既有 managed 报告供独立登记，进度不依赖其生成或大小。Plan 自定义目标自动接入延后。

## 原基线验证记录

- 固定 L1 首轮与最终轮通过；最终报告：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261024T004635.026453800Z/summary.json`。
- 固定 L2 首轮与最终轮通过；最终报告：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261024T004654.453001800Z/summary.json`，场景与资源清理均完整。
- 新 L3 `testing.runtime-progress` 两轮通过；最终报告：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261024T004654.430212900Z/summary.json`，实际场景 201.36 秒，清理完整。覆盖初始化、通过、重跑清零、非法内容/断连保留快照、恢复/重启、原请求及业务 revision 不变、无 managed 历史及停止后查询。
- owner 定向 Progress/候选冻结/查询及 SQLite 断言通过；混合 Go/PowerShell 八进程写入通过。最后补充的 closed 原 Run 显式恢复后重新采集断言通过。
- 扩展 owner 回归中 runtimeprogress、testmanagement、runnerclient、SQLite、test-feedback 包通过；internal/app 出现下文所列已有失败，并达到整包默认 10 分钟时限，未完成整包回归。
- Web build 通过；完整客户端 27 文件、179 测试通过。最后展示文案调整后构建与进度组件 4 项重验通过。
- 业务项目 `tools/testing/test-runtime-progress.ps1` 35 项协议断言两轮通过，最终证据：`C:/Users/lmas1/AppData/Local/Temp/merge-progress-check-fed86b0e350f45cba7804acda3b11730/proof.json`；`test-managed-reports.ps1` 42 项原报告断言通过，现场：`C:/Projects/merge-h-308-code/Test-Log/testing/managed-protocol-23d637afb97847dd91bb31a2b768ed5a`。
- 真实链入口适配后的 `TestTMFixtureProgramEmitsActualPassAndFailure` 通过，实际 pwsh 断言程序先 pass 后 fail，核验输出、原报告与进度字段；live package 编译通过。未运行真实模型/浏览器现场，不将这些结果计为真实 L4 通过。

## 首轮失败和边界

新 L3 首轮在测试夹具提交新材料时使用示例远端地址，Git 在鉴权前明确失败；原场景及清理证据保留于 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261024T003637.790352500Z`。修复为本场景自有本地 Git 服务后使用新现场重跑。

前端首轮完整测试中一项仍期待旧报告引用，调整为进度展示断言后完整 179 项通过。真实链夹具首次改造漏复制共用脚本，定向测试明确失败；补齐夹具和 PowerShell 7 依赖后实际通过，不将首次失败计为通过。

扩展 `internal/app` 回归复现 `TestHTTPConsumerBoundary`（依赖闭包触达 runnercontract）和 `TestInstallLaunchersWritesCRLF`（原断言未包含 restart.bat），两项均已在[既有迁移验收](steerline-flow-layout.md#同步最终主干及复验)记录。整包随后达到默认 10 分钟上限，超时时正在运行 `TestRunnerPresetFreeze/accept_replacement_after_explicit_restart`。本次修改相关的 app 定向断言通过，不将扩展整包结果记为通过，也未扩大范围修改这两项既有断言。

Runner 的真实远端采集及原生恢复资格见其原专项验收；本仓 L3 为调用方 controlled 联验。此次未运行 Unity，业务项目协议检查不代表 L1–L4 业务测试全量通过。安装部署、旧库及既有候选恢复不在本次验证范围。

## 提 PR 前质量审计与主干同步

本聊天直接审计两个仓库的最终差异，未启动独立审计 agent。确认并修复以下问题：

1. 最终 E2E 汇报原先只核验匹配事件和部分场景字段，比执行中的 `checkE2EGroup` 宽松，可能将缺资源证据或有意外跳过的结果完成。最终回写现复用同一完整校验，新增通过/缺资源/跳过/缺清理四组断言。
2. 终态快照缓存原先只看业务 status，遗漏 Runner 已核实的 `stop_status=stopped` 和 `closed`。现按公开收尾状态保留缓存；原 Run 恢复 active 后继续观察。定向断言覆盖业务 running 而资源 stopped 的情况，L3 增加停止后 closed、删除来源及再次重启的完整检查。
3. 恢复被误替换的两项原输出路径过滤测试，在保留原断言基础上增加 TestMain 环境隔离；移除无消费者的候选历史/日志/失败计数字段。
4. rebase 到 `65a51586`，逐处保留主干 Git JSON 历史与配置导航；不恢复其已删除的数据库历史、register 或复验关系。schema 因主干已占用 30 而顺延为 31；同步路径清单和产品文档。

审计后验证：

- 固定 L1：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261024T010706.218721600Z/summary.json`，通过。
- 固定 L2：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261024T010727.218962900Z/summary.json`，通过、清理完整。
- 增强后的 L3：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261024T010851.158548700Z/summary.json`，通过、清理完整，含终态保留反例。
- `go test ./... -run '^$'` 全仓编译检查通过；runtimeprogress、testmanagement、runnerclient、SQLite、test-feedback owner 包完整通过。最后恢复的原测试、最终进度判定及 app/真实 pwsh 夹具定向重验通过。未重跑存在已知失败的 app 整包，不声称全量 Go 行为通过。
- 同步主干后 Web build 与完整客户端 27 文件 186 项通过；最后 DTO 清理后的构建与进度组件 4 项重验通过。
- 业务项目新增清单改变/恢复及源码 commit 改变断言后 38 项通过：`C:/Users/lmas1/AppData/Local/Temp/merge-progress-check-6746b80a6b374c58a96b511bd18aaca9/proof.json`；原报告协议重验 42 项通过：`C:/Projects/merge-h-308-code/Test-Log/testing/managed-protocol-bc3a8416c31a43c6be56207ee0a66d6d`。
- 文档链接检查：573 份 Markdown、3587 个本地链接，0 错误；两个仓库 `git diff --check` 通过。

审计中的命令修正：首次前端调用误用不存在的 `test:client`，改用正式 `npm test`；首次 L3 在 PowerShell 中参数未正确传递而未启动，改用引用参数后新现场执行。DTO 清理曾留下一个旧 fixture 字段导致 TypeScript 编译失败，清除后构建通过；这些失败尝试不计为通过。原基线报告仍保留，不据主干同步改写其测试来源。

业务项目 PR：[merge-h-308-code #414](http://192.168.0.66:3000/zymobile/merge-h-308-code/pulls/414)。本次只提交源码，安装和 Task 18 不变；Plan 自动目标、真实模型/远端链及 Unity 业务通过仍按上述边界，不由协议断言代替。
