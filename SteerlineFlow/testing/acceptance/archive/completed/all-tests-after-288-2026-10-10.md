# #288 后全项目回归与过期测试修复

按用户直接授权 rebase 到 `6d4b4fa2d67b77156eeda64d103ce71c95f3450b`（PR #288），运行全部测试入口并修复过期期望、夹具及等待条件。交付 [PR #289](https://github.com/erranliu/SteerLine/pull/289)，不新增 roadmap 任务。全部改动属于测试、测试基础设施或文档；产品运行代码、权限设置和安装环境未修改。

## 修复与保留的断言

- 内置目录期望由 v5 更新为当前 v6，保留目录/进程宿主实例隔离与无探测副作用检查。
- adapter 守卫仅准入纯 `workerreturn` 回复协议；新增间接 SQL、执行应用、任务实现与产品查询负例，继续检查 Windows/Linux 和 inactive-tag 依赖。
- 活动读取适配当前持久化状态事件，验证原 Run/Execution/Attempt 隔离和游标续读无重复，不再要求过期的空页诊断。
- L1 移除已无测试的旧 protocol 辅助包，保留严格零跳过/零遗漏检查；发布 helper 匹配当前“使用检出分支”的提示，分步本地提交、只读报告、重复/乱序拒绝与精确 head 断言保留。
- 服务发现等待覆盖原生 preflight 和服务工厂；双候选查询保留五分钟总限，控制台反馈准备覆盖 execute/report 两轮，原身份、就绪、锁、回执和草稿断言保留。
- 原生工作区场景使用当前连接文案；工作区操作、两次 ConPTY、重启和第二条三阶段工作流单独用十分钟业务限/十一分钟 Go 父限，普通命令限与清理判定不变。
- startup 诊断读取当前 startup.json；fixture Git 错误保留原始原因。文档删除已不存在的探针/恢复命令并补充 WinUI 与独立调试入口。

## 实际结果

| 范围 | 结果 |
| --- | --- |
| Full，三 worker | 1360 条父子记录，0 失败，17 默认 skip；passed/complete=true，约 47 分钟 |
| L1 | 523 条父子记录，0 失败、0 skip；passed/complete=true |
| tagged 基础设施/边界 | 202 条记录，0 失败，3 默认 opt-in skip |
| 当前注册 L2/L3 | 8/8 场景通过，全部 cleanup_complete=true；三个真实 Codex 场景核验正数 usage |
| PowerShell 默认路径防护 | 10 项通过，未选择需机器权限的 symbolic-link 项 |
| 独立 debug 包 | 两包隔离/模拟/重置与真实脚本 PR 验证通过；[测试 PR #213](https://github.com/erranliu/SteerlineTest/pull/213) 已关闭，专属 base/head 清理完成 |
| WinUI | 单元 19/19；真实工程编译 0 warnings / 0 errors；不声明窗口人工验收 |
| Python | roadmap-runner 18/18，任务身份脚本 10/10；未执行 roadmap 调度 |
| Codex 沙箱双任务原生回归 | **未通过**：第一任务的 probe、三个原生阶段及真实合并交付完成；第二任务在 implement.report 正确阻断，原因见下 |

完整 L2 的真实 [SteerlineTest PR #212](https://github.com/erranliu/SteerlineTest/pull/212) 已合并；head `9a4e2a3ba04b714a4664254071f1ff2f10fe1df1`，merge `18b84593e335a454cd457cd1aa01324cecb62a9c`。L3 local 的 provider 为 loopback，模型真实。Orca native 场景不启动模型，不作为 Orca 真机模型交付重跑。未批量启用其他专属原生探针，default skip 不计通过。

## 原生自动审核拒绝

命令 `scripts/test-codex-sandbox.ps1` 使用全新 `C:\SteerlineDebug\dev-20261010-222122-7164baba`。第一任务 [PR #214](https://github.com/erranliu/SteerlineTest/pull/214) 已合并，head `a0100e9b0527c9ed79fb36f4b23092e47eabc466`、merge `d3163c87f90fe72cc58c703af56ffb79935ecf33`；中心任务 1 已交付并解锁任务 2。

第二模型在 implement.execute 提前尝试 publish.ps1，违反模板的阶段要求。普通沙箱 Git 返回 SEC_E_NO_CREDENTIALS；同一发布命令的原生自动审核拒绝重试，因为未将具体 GitHub 目的地及载荷认定为已授权的可信敏感数据外发。模型随后在报告轮返回 blocked/permission，驱动如实失败。未修改权限、代答、重试被拒动作或把拒绝改成 skip/pass；不宣称双任务完成。保留 sandbox-test.log、sandbox-live.json 和原回合 journals。测试服务已停止；第一 PR 已核验 MERGED，本次专属远端分支由驱动清理。

## 源码与证据

[结果索引](all-tests-after-288-2026-10-10.json) 逐份记录源码、选定范围、默认 skip、hash 和实际状态，核对当前八项注册表无遗漏/重复。最终 Full、native、路径/debug 入口来自 clean `7164baba9309dc1f19635beb521de8fda5220b3f`；L2/L3 controlled/live 来自 clean `4ca00a3f54f4d3c0610d82c27cd470c799ffb524`；L1 来自 clean `38ab65ce799ae3506d2940e3f7546fc4c9cde416`；基础设施来自 clean `cbb982039b2803b74e5a5edcb3aae90721ed31d2`；WinUI/Python 来自主线基线。不同测试源码间仅有测试基础设施与文档差异，不伪装为同一源码重跑。

原始目录：`C:\Users\lmas1\AppData\Local\Temp\steerline-all-tests-20261010-190239643`；debug 原生证据位于各生成包 evidence。仓库只保存最小摘要与结果索引，不提交完整日志、数据库、凭据或 prompt。

首次 Full、第二次 Full、基础设施/L1、旧发布提示门槛、旧控制台文案及 native 五分钟截断均保留诊断，不计最终通过。首轮两次夹具 Git 验证失败单独复验及连续三轮共九条记录通过，诊断已补原始错误，不认定未复现故障根因已修复。临时 PowerShell 5 编排曾把 Go stderr 的通过摘要误判为异常；正式基础设施 JSON 已通过，修正编排按退出码继续，保留原编排失败。

最终文档导航、任务身份及差异格式检查见交付记录。静态检查不代替原生或人工验收，临时目录不承诺永久保留。
