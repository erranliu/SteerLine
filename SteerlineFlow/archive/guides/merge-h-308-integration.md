# merge-h-308-code 接入（历史记录）

> Historical record from the earlier Beads-backed integration. Its dedicated config sample and completion adapter were retired by [Beads-path removal](../spec/completed/beads-path-removal.spec.md); do not follow the setup steps below as current operating instructions. Current setup uses explicit native authority, fixed project steps and center-owned delivery. See [native config](../../../examples/config.json), [worktree preparation](../../../docs/product-design/worktree-preparation.md), and [hosting](../../../docs/architecture.md).

目标：`C:/Projects/merge-h-308-code`，Gitea `http://192.168.0.66:3000/zymobile/merge-h-308-code`，远端 origin，主线 AIRefactor（来自仓库 `.codex/gitea.json`）。检查版本为 bd 1.1.2、codex-cli 0.145.0、Gitea 1.27.1。

## 已实现的专用接入

| 组件 | 仓库内路径 | 职责 |
| --- | --- | --- |
| start | `.agents/skills/steerline-start-task/SKILL.md` | 核验已领取派发、worktree/Beads，Spec → Plan → 实现和验证 |
| finish | `.agents/skills/steerline-finish-task/SKILL.md` | 提交、推送、创建/复用并核验 PR，返回结构化结果，保留 in_progress |
| completion | `tools/Beads/Complete-SteerlineTask.py` | 接收已核验的合并事实，复用生命周期脚本，记录 PR/run 并核验 BV |
| adapter tests | `tools/Beads/tests/test_steerline_completion.py` | 用临时目录和替身验证副作用边界及部分成功重试 |

手动 start-sl-task、finish-beads-task、start-bd-task 和既有生命周期脚本保留原用途。新 start/finish 在一次 codex exec 中串行执行；只有服务的 PR 轮询确认合并后才调用 completion。

当时的 `examples/merge-h-308-code.json` 指向上述旧文件并启用了 completion 命令。该配置样例已移除，不再代表当前协议。

## 真实启动前

1. 将新技能、适配脚本和测试审阅并合入 AIRefactor。它们目前是本地新增文件；新 worktree 只取得远端主线文件，不复制未提交内容。本地 check 通过不证明文件已经发布。
2. 确认 Python 3、pwsh、bv、bd、Codex 原生 exe 可用。适配器使用 Python 标准库；既有生命周期通过 PATH 调用原生 bd，不支持 bd 的额外前置 args。
3. 启动环境设置 STEERLINE_GITEA_TOKEN 为服务的 PR 读取凭据；该私有仓库匿名访问曾返回 404。token 不写配置或 runtime。创建 PR 所需的 tea 登录独立沿用仓库配置。
4. 确认 Codex profile 允许访问 worktree、共享 Beads、共享 Spec 和必要工具。pr-gitea 的 tea 包装器有 Windows 控制台要求；本次未用真实模型验证无人值守出版，权限或控制台不满足时技能返回 blocked。
5. 构建后先执行 `steerline check -config examples/merge-h-308-code.json`，再执行 run。run 会真正领取业务任务并调用模型。

## 验证范围

技能通过 skill-creator 格式验证。Python unittest 使用临时替身，覆盖错误 PR/领取者/前缀/数据库无写入、拆分父任务和后继验证、生命周期部分成功重试、关闭后 BV 失败重试、拓扑/视图错误拒绝完成。

Steerline 测试覆盖 PR 未合并不调用 completion、回执 task/run/PR 错误不完成、回执成功但 Beads 未关闭不完成，以及重启后保留派发时的命令并重试尾部。请求和回执保存于运行目录，可查看部分已应用操作。

本次创建技能不执行其业务工作流：没有领取/关闭业务任务，没有真实模型开发、创建/合并业务 PR 或真实结项。已有 Beads 运行文件不纳入技能交付。

当时的约定见 [技能契约](skill-contract.md) 与 [worktree 准备](../../../docs/product-design/worktree-preparation.md)；结项协议已退役。
