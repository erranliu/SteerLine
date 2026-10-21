# Orca supervised 双目标验收

> 归档于 2026-10-21：退役方案，原失败和未验项保留，不作为当前补验或实施队列。当前接入见[入口](../../../integration/steerrunner.md)。固定 commit/原路径及证据内容继续按原记录取证。

仅归属本聊天“扩展原远端链路支持本机，保留并切断原本机链路”，无 roadmap Task ID。[Plan](../../../archive/spec/retired/orca-supervised-targets.plan.md)定义当前范围和命令；structured 延期及 PR #265 独立。

| ID | 条件 | 状态/证据 |
| --- | --- | --- |
| OST-A1 | 本机目标无远端路由；原 worker 身份、续轮、问答/完成/恢复；原远端行为保留 | 通过；adapter 包最终复验 2.473s；本机新测试 5 项及其身份/预检子场景；现有远端确定性测试保持通过；opt-in live 未选 |
| OST-A2 | 单一 supervised 注册支持两目标；旧本机 host/codec/protocol 切断，源码保留 | 通过；app 整包 110.281s；新增注册断言、原配置选择/冻结/预检回归。增加本机 status target 身份校验后，受影响 4 项再验 10.644s；TM06 生产资源接线 8.245s |
| OST-A3 | 依赖边界、native/testenv 当前入口 | 通过；boundary 12.219s、e2e boundary 4.015s、native 1.457s、testenv 1.185s、protocol 最终 0.908s、runner 16.517s；保留 Orca adapter 单测 4.950s；产品构建通过 |
| OST-A4 | 文档和空白检查 | 通过；296 文件、1842 本地导航链接、0 错误，git diff --check 通过（仅换行提示） |

真实模型、安装、运行服务与历史执行迁移未选择。原生 Enter 提交问题不由本次配置目标修改解决，不把 fixture 通过当作真实模型验收。

## 实现与证据

2026-10-09，分支 `codex/orca-supervised-local-target`，首次验证基线 commit `d475684193195965401e252c417d114785188ad4` 加本次实现差异。证据为本聊天对应 Go/构建/静态检查输出；首次构建文件在 `C:/Users/lmas1/AppData/Local/Temp/steerline-supervised-local-20261009.exe`，未安装。提交前最新主干复验另见下节，不用首次结果代替更新后的结果。

`orca.supervised-profile` 格式2 增加 target，local 严格拒绝远端连接字段；environment/省略 target 保持原远端入口。local 预检还拒绝环境变量将 CLI 重定向到远端、缺 target 证明、错 repo 或 runtime，均在输入前阻断。本机 Runtime/terminal/worktree 身份字段已通过只读查看原 105 Dispatch 的公开 worker-show 核对，仅查看未改动其资源。

两 target 使用同一 `orcaremote.ExecutionLifecycle`。保留取消不支持、交付后保留资源和中心停服只解除观察；完成材料仍按原 supervised 的消息/结算语义。没有接入 PR #265 的旧本机 observer 或原完整结果文件协议。

旧本机产品 host、profile codec、固定 protocol、工厂和 conversation reader 接线切断，原模块源码未修改。旧本机三个 live、四个 startup 和 candidates.local 场景已移出 suites.json，驱动代码保留；旧产品 integration fixture 明确 retired/skip，不计当前通过。原远端 `delivery.orca-remote-codex` 登记保留，本次没有运行该真实场景。

structured 方案/探测草稿仍未注册，保留推迟状态，排除于本 PR。未启动新真实 worker、未合并本次改动、未修改运行库，也未改动安装服务。

## PR 提交前最新主干复验

本次提交已 rebase 到 `267858f0`（#265、#262 已合并）。OST-A1 通过（orcaremote 3.002s）；OST-A2 通过（app 110.358s，TM06 8.661s）。OST-A3 中 boundary 12.502s、e2e boundary 6.308s、protocol 1.485s、native 1.455s、runner 21.415s、保留 Orca 模块 4.654s 与产品构建通过。构建文件为 `C:/Users/lmas1/AppData/Local/Temp/steerline-supervised-pr-20261009.exe`，未安装。OST-A4 通过（293 文件、1824 本地导航链接、0 错误；PR 差异空白检查通过）。

最新 testenv 包有一项主干已有失败：`TestCandidatePublicationFailureRequiresCompletedOriginalFeedback` 在 `candidates_wait_test.go:13` 报 completed original feedback without PR was not diagnosed。该测试输入 status=waiting_input，当前 helper 判断 waiting_decision；两文件与主干完全相同。从 origin/main 导出独立源码，在 `C:/Users/lmas1/AppData/Local/Temp/steerline-supervised-main-check-7rye58hg/source` 单独执行同名测试，也以相同错误失败（0.915s）。未扩大本 PR 修改候选反馈测试；最新 OST-A3 记为相关检查通过、testenv 存在主干已有阻塞，不能标最新整组全部通过。
