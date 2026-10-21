# 全量回归与过期测试维护

> 归档于 2026-10-21：已交付工作的历史契约或验收，结果仅适用于原固定版本。当前文档见[入口](../../../README.md)。固定 commit/原路径及证据内容继续按原记录取证。

用户授权本聊天 rebase、运行全部测试、修复或删除过期测试并提交 PR。普通开发维护，不占 roadmap Task ID，不更新执行 DAG。

## 当前状态

已 rebase 到 `76ce0bda`（主干合入 HTTP 请求生命周期、worker 三层重构及 Web 前端）。2026-10-15 完成终轮：Full complete / passed，1771 条父/子记录中 1752 通过、19 默认跳过、0 失败；当前 26/26 登记场景通过且清理完成，全部最新场景证据源 commit 均包含该主干。验证代码 commit 为 `640ad30b`，后续仅登记证据与导航，未修改实现。交付 PR：[SteerLine #315](https://github.com/erranliu/SteerLine/pull/315)，已合并；合并后的再次 rebase / Full 与新增 SQLite 取消泄漏修复见[后续记录](sqlite-cancellation-full-2026-10-15.md)。

最初 `ab814da9` 的 Full 已完整执行：1697 条父/子测试记录，16 条失败、21 条默认跳过。随后新主干的复验发现新增准入检查不接受旧夹具，另有 HTTP 服务启动等待不足；失败日志保留，不改写为通过。

## 修复及退役范围

- 已确认原问题回复后，保留的 QuestionRoute 不能继续使恢复状态被分类为 question。仅 pending / answer_saved 决定仍待回答；既有未知回复和原请求守卫保留。
- 更新缺原生 final 的失败处理断言、同一执行不能替换已关闭 PR 的断言，以及 execute/report 两组原会话回合断言。
- 原生 fake 返回原 thread cwd/turns、worktree/terminal incarnation；进程输出 helper 不再重建无关 CLI。控制台反馈明确选择原 P1，保留旧实例明确拒绝后的有界刷新。
- E2E 回答驱动等待原 Command 的 accepted 事实；pending、未知或拒绝不冒充提交成功。恢复控制台通过活动 HTTP 服务读取，不从离线数据库假定已取得报告。
- 候选停止夹具绑定正式发布目标和原工作区意图；异步夹具声明真实所需的准入事实。ER4 保留原准备期限、重启、晚成功和正式重验，移除已有原会话时人为再次 prepare_session 的无效断言。原操作丢回执、只读对账和禁止重放继续由生命周期 checkpoint / Codex 原回执测试覆盖。
- 删除旧 detached app-server LEGACY 两项 live 测试、未登记的 candidates.local、本机 Orca live 恢复入口。根据[当前部署配置记录](../../remote-only-orca-profiles.md)，底层 local/environment 隔离夹具和无模型原生工作区能力样本保留。
- 调试包碰撞测试按当前 checkout 的 commit/dirty 后缀构造目标；HTTP 服务启动测试有界等待完整启动路径，未改变产品期限。

## 最终验证

| 范围 | 结果与证据目录 |
| --- | --- |
| Full | 三 worker 完整枚举、无遗漏/重复；1771 条记录、1752 通过、19 默认跳过、0 失败；准备 20.5 秒、执行 2812.9 秒；`full-03` |
| L1 | 先前固定集 553 条记录通过；`l1-01`。终轮 Full 覆盖其确定性用例，不以旧 L1 替代新主干完整回归 |
| E2E 基础设施 / 编译 | 新主干带 tag 基础设施 181 条记录、4 条默认跳过、0 失败；`tagged-infrastructure-03`。终轮已编译全部登记场景，首次全包带 tag 编译证据为 `tagged-compile-01` |
| L2 | controlled 门槛和真实 Codex smoke 2/2，通过且清理完成；`l2-03` |
| L3 controlled | 19/19 当前场景通过且清理完成；`l3-recovery-02-2/3/4`、`l3-delivery-runtime-02` 与 `l3-recovery-03-1/2/3` 的每场景最新结果。此前失败/超时已单独复验，不改写历史失败 |
| L3 live | 交付、等待合并/完成后重启、真实同 thread 新 Attempt 恢复 3/3，通过且清理完成；`l3-live-03` |
| 双任务沙箱 / Git | 新生成包的两套独立 worktree/session，各三阶段原生命令、正数 usage、自动审核、精确真实 PR 合并和 2 次唯一交付通过；`l3-sandbox-03`，未代答审核，清理完成 |
| L3 native | 真实 CLI 原工作区创建/删除、查询、重启及无关任务启动通过；`l3-native-03`。只验证资源能力，无 Orca 模型输入，清理完成 |
| Web | 构建、5 文件 26 个单元测试、HTTP owner 回归通过；`web-01`。新独立 simulate 包真实 Edge smoke 1/1，通过并正式停服、核实进程退出/锁释放；`web-browser-01.log` / `web-stop-02.log` |
| 独立 debug live | 生成、模拟、隔离、重置和脚本 PR 验证通过；`debug-live-03.log` |
| TUI HTTP | 真实生成包入口通过且 complete / cleaned；`tui-http-02-evidence` |
| 路径隔离 | 默认脚本入口通过；`runtime-path-01-evidence`。未启用需要管理员符号链接权限的可选参数 |
| 静态 | 两组 Python 测试 10/10、18/18 通过；文档链接、任务身份及差异检查通过，最终数目见静态检查日志 |

原始证据根：`C:\Users\lmas1\AppData\Local\Temp\steerline-all-tests-20261014-181007434`。日志与凭据不提交仓库；[最小结果索引](full-regression-2026-10-14.json)登记源码身份、42 份报告摘要、26 个当前场景与退役场景的区分、报告/场景 SHA-256。原始日志不复制到仓库。Windows PowerShell 直接继承 Core 模块路径时曾不能加载 Security 模块；脚本使用本次进程的机器模块路径完成复验，未修改全局设置。

## 真实 PR 与边界

本次 SteerlineTest 专属 base/head 测试 PR：[241](https://github.com/erranliu/SteerlineTest/pull/241)、[242](https://github.com/erranliu/SteerlineTest/pull/242)、[243](https://github.com/erranliu/SteerlineTest/pull/243)、[244](https://github.com/erranliu/SteerlineTest/pull/244) 已真实合并；[245](https://github.com/erranliu/SteerlineTest/pull/245) 是脚本发布验证，清理关闭，专属分支删除。终轮 [246](https://github.com/erranliu/SteerlineTest/pull/246)、[247](https://github.com/erranliu/SteerlineTest/pull/247)、[248](https://github.com/erranliu/SteerlineTest/pull/248)、[249](https://github.com/erranliu/SteerlineTest/pull/249) 均真实合并，专属分支与本次资源清理完成；head/base/merge SHA 见索引及 sandbox 原始证据。

所有产品调试只用新生成的 `C:\SteerlineDebug` 包；分级回归使用隔离夹具，未使用或修改安装环境。停用本机 live 的失败记录属于历史，不能当作当前登记场景失败或通过。远端 environment 受控验证不代表真机恢复：当前注册表没有新版远端真实恢复入口，旧单轮远端驱动不重新准入。默认原生 opt-in 跳过项照实保留，不算通过。
