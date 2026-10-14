# 全量回归与过期测试维护

用户授权本聊天 rebase、运行全部测试、修复或删除过期测试并提交 PR。普通开发维护，不占 roadmap Task ID，不更新执行 DAG。

## 当前状态

已 rebase 到 `76ce0bda`（主干合入 HTTP 请求生命周期、worker 三层重构及 Web 前端）。最终完整回归和分级场景复验尚在执行，当前不能声明全量通过；PR 尚未提交。

最初 `ab814da9` 的 Full 已完整执行：1697 条父/子测试记录，16 条失败、21 条默认跳过。随后新主干的复验发现新增准入检查不接受旧夹具，另有 HTTP 服务启动等待不足；失败日志保留，不改写为通过。

## 修复及退役范围

- 已确认原问题回复后，保留的 QuestionRoute 不能继续使恢复状态被分类为 question。仅 pending / answer_saved 决定仍待回答；既有未知回复和原请求守卫保留。
- 更新缺原生 final 的失败处理断言、同一执行不能替换已关闭 PR 的断言，以及 execute/report 两组原会话回合断言。
- 原生 fake 返回原 thread cwd/turns、worktree/terminal incarnation；进程输出 helper 不再重建无关 CLI。控制台反馈明确选择原 P1，保留旧实例明确拒绝后的有界刷新。
- E2E 回答驱动等待原 Command 的 accepted 事实；pending、未知或拒绝不冒充提交成功。恢复控制台通过活动 HTTP 服务读取，不从离线数据库假定已取得报告。
- 候选停止夹具绑定正式发布目标和原工作区意图；异步夹具声明真实所需的准入事实。ER4 保留原准备期限、重启、晚成功和正式重验，移除已有原会话时人为再次 prepare_session 的无效断言。原操作丢回执、只读对账和禁止重放继续由生命周期 checkpoint / Codex 原回执测试覆盖。
- 删除旧 detached app-server LEGACY 两项 live 测试、未登记的 candidates.local、本机 Orca live 恢复入口。根据[当前部署配置记录](remote-only-orca-profiles.md)，底层 local/environment 隔离夹具和无模型原生工作区能力样本保留。
- 调试包碰撞测试按当前 checkout 的 commit/dirty 后缀构造目标；HTTP 服务启动测试有界等待完整启动路径，未改变产品期限。

## 已完成验证

| 范围 | 结果与证据目录 |
| --- | --- |
| L1 | 原集 553 条记录通过；`l1-01` |
| E2E 基础设施 / 编译 | 167 条记录、4 条默认跳过；带 tag 包编译通过；`tagged-infrastructure-01` / `tagged-compile-01`，新主干终轮待完成 |
| L2 | controlled 门槛与真实 Codex smoke 均通过，清理通过；`l2-01`，新主干终轮待完成 |
| 真实 Codex 交付 / 恢复 | 三条 live 场景通过；`l3-live-01`，新主干终轮待完成 |
| 双任务原生沙箱 / Git | 两次独立三阶段执行、正数 usage、真实 PR 合并和清理通过；`l3-sandbox-01`，新主干终轮待完成 |
| Web | 构建、5 文件 26 个单元测试、HTTP owner 回归通过；`web-01`。新独立 simulate 包真实 Edge smoke 1/1，通过并正式停服、核实进程退出/锁释放；`web-browser-01.log` / `web-stop-02.log` |
| 独立 debug live | 生成、模拟、隔离、重置和脚本 PR 验证通过；`debug-live-03.log` |
| TUI HTTP | 真实生成包入口通过且 complete / cleaned；`tui-http-02-evidence` |
| 路径隔离 | 默认脚本入口通过；`runtime-path-01-evidence`。未启用需要管理员符号链接权限的可选参数 |
| 新主干失败定向复验 | 原意图候选停止 1/1、HTTP 服务启动两项 2/2 已通过；`candidate-stop-06` / `api-startup-03`。ER4、checkpoint 和异步原/过期回执分项已通过，仍待最终 Full 汇总 |
| 静态 | 文档链接 407 文件 / 2562 链接无错；任务身份检查与两组 Python 测试 10/10、18/18 通过；最终登记后再检查 |

原始证据根：`C:\Users\lmas1\AppData\Local\Temp\steerline-all-tests-20261014-181007434`。日志与凭据不提交仓库；最终汇总将登记报告摘要与 SHA-256。

## 真实 PR 与边界

本次 SteerlineTest 专属 base/head 测试 PR：[241](https://github.com/erranliu/SteerlineTest/pull/241)、[242](https://github.com/erranliu/SteerlineTest/pull/242)、[243](https://github.com/erranliu/SteerlineTest/pull/243)、[244](https://github.com/erranliu/SteerlineTest/pull/244) 已真实合并；[245](https://github.com/erranliu/SteerlineTest/pull/245) 是脚本发布验证，清理关闭，专属分支删除。后续终轮事实另补登记。

所有产品调试只用新生成的 `C:\SteerlineDebug` 包；分级回归使用隔离夹具，未使用或修改安装环境。停用本机 live 的失败记录属于历史，不能当作当前登记场景失败或通过。远端 environment 受控验证不代表真机恢复：当前注册表没有新版远端真实恢复入口，旧单轮远端驱动不重新准入。默认原生 opt-in 跳过项照实保留，不算通过。
