# merge-h-308-code 实际接入检查

检查日期：2026-09-17。只读检查，未领取、关闭业务任务，未调用真实模型，未修改技能，未创建/合并 PR。

| 项目 | 实际发现 |
| --- | --- |
| 仓库 | `C:/Projects/merge-h-308-code` |
| origin | `http://192.168.0.66:3000/zymobile/merge-h-308-code.git` |
| 主线配置 | `.codex/gitea.json` 的 `default_base_branch = AIRefactor`，与 origin/HEAD 一致 |
| Beads | 本机 bd 1.1.2；真实 ready/show/list JSON 成功解析；未初始化数据库 |
| Codex | 本机 codex-cli 0.145.0；exec help 支持 JSON 事件、output-schema、output-last-message、stdin |
| Gitea | `/api/v1/version` 返回 1.27.1；匿名查询私有仓库返回 404，需认证 |
| start | `.agents/skills/start-bd-task/SKILL.md` |
| finish | `.agents/skills/finish-beads-task/SKILL.md` |

`examples/merge-h-308-code.json` 是按现状生成的接入草案，**当前预期无法通过技能契约检查**。不要直接将它当作已经启用的生产配置。

## 已确认的契约差异

- `start-bd-task` 只接收数字后缀，在其步骤 5 自己执行 `bd update --claim`。需要一个显式接受完整调度上下文的已领取模式，核实 claimant 后跳过重复领取。现有前缀校验、数据库定位和 BV 约定由仓库维护。
- `start-bd-task` 在刷新 BV 后即停止，不执行 Spec/开发。适配入口必须明确衔接仓库实际开发流程；服务不猜测。
- `finish-beads-task` 要求 PR **已经合并**，随后调用 `tools/Beads/Invoke-BdTaskLifecycle.ps1`，处理结项、拆分树、后继状态和 BV。它不是 M0 所需的提 PR 后返回入口，不能直接绑定成 finish。
- 仓库存在 `.codex/skills/mrac-auto/SKILL.md`，其职责包含开发到 PR。应由维护者确定是否将该既有流程封装成 finish 接入入口，并确认它能在等待人工合并时返回；不把 MRAC 内部实现复制进调度器。
- 仓库的手动 `worktree-start` 技能仍指定 `AIRefactor_5`，不同于 PR 配置的 `AIRefactor`。接入入口必须使用显式传入的主线，不调用会忽略它的旧默认流程。本次按 `.codex/gitea.json` 生成草案，没有切换业务分支。
- 当前 finish 的自定义结项事务不能被 M0 的普通 `bd close` 完整替代。尤其是 split 聚合收尾、blocked 后继变为 open、BV 刷新。**在维护者确定采用原生依赖结项的接入范围，或完成独立的生命周期适配之前，不启用真实自动调度。** 单纯替换技能名字不足以解决这一差异。

## 最小接入调整

1. 在目标仓库中由维护者添加/适配两个入口，落实 [技能契约](skill-contract.md)，保留既有流程。完成后才写入 v1 标记。
2. 明确 M0 接管任务使用原生 Beads 依赖与 close 语义，并只选择这类可执行类型；现有自定义拆分生命周期先继续人工运行。若无法按类型隔离这类任务，则该仓库暂不适合启用 M0。
3. 配置草案的两个路径和 invocation 指向真正适配后的入口，核实目标主线。
4. 在启动环境中设置 `STEERLINE_GITEA_TOKEN` 为该仓库的只读查询凭据。服务不会写 token 到配置或 runtime。Codex 内部创建 PR 所需的 tea 登录沿用目标仓库原有方式，属于独立的开发流程权限。
5. 运行 `steerline check -config ...`。只有检查通过后才运行 `run`；`run` 将真正领取业务任务并调用模型，验收期间不要运行它。

启动检查不会创建、迁移或修复 Beads 数据库。现有 `beads.role not configured` 提示保留原样，不替用户修改 Git 配置。
