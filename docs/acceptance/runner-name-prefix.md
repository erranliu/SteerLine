# Runner 工作区命名前缀验收

归属[本次 Plan](../plans/runner-name-prefix.plan.md)，不占 roadmap Task ID。

| 项目 | 范围 | 结果 |
| --- | --- | --- |
| NP-A1 | 同事务分配 Number 并保存前缀（C1 为 `sl-<task_id>`，C2 起追加 `-c<Number>`）；服务返回最终冻结提交 | 通过：C1 不带候选后缀，C2 带 -c2，与 Number 一致，最终提交从服务路径读回 |
| NP-A2 | 后续候选分配、重复请求与保存更新不改变原名称/提交 | 通过：第二候选分配后首请求重放逐值相同；保存修改前缀被拒绝 |
| NP-A3 | 实际冻结输入经本次 Runner EXE 的公开 validate 接口受理 | 通过：`TestRunnerPresetFreeze/published-runner-name-prefix` 非零执行，公开 CLI 返回 ok/valid true |
| NP-A4 | Runner 准入/schema、有效前缀冻结、本机/远端 Orca 创建参数与回执及只读恢复 | 通过：Runner 定向受控验证，含缺省 sr、通用前缀、非法输入、名称冲突和恢复零重建 |

2026-10-21 最终 C1 规则复验，Windows/amd64：SteerLine 测试基线 `36d4f5967e9ecc2a9e02b0f124e8bbf3aff77d50`，Runner 基线 `8261310d5f2d1edb6fa0c81ba05fa4119b561386`，验证时均包含本次修改。SteerLine 基线树与 `origin/main` 相同，提交时仅迁移本次提交到远端主干基线，避免纳入本地历史合并提交。Runner 从独立工作区构建，仅包含本功能，不纳入主工作区的其他在途修改。最终证据目录为 `C:/Users/lmas1/AppData/Local/Temp/steerline-name-prefix-c1-20261021-1792550428937/`，含 `summary.json`、两仓 `*-tests.jsonl`、本次构建的 `steerrunner.exe`。EXE SHA256 为 `df285e77318f3858ddf9d2f588be263a612ba1c03c69167bbd2fae054ee7090e`。先前全部候选带后缀的验证保留在 `steerline-name-prefix-20261021-1792549733835/`，不作为最终 C1 规则的证据。

按 Plan 命令执行：SteerLine 三包 26 个顶层测试通过，含子用例 45 次 pass，零 fail/skip；Runner 四包 19 个顶层测试通过，含子用例 168 次 pass，零 fail/skip。跨仓检查显式设置 `STEERRUNNER_EXE=<上述目录>/steerrunner.exe`，仅运行公开纯校验，不启动服务。两仓 `git diff --check` 通过；SteerLine 文档导航检查 504 文件、3111 本地链接、零错误。

首轮 Runner 新增内容 schema 测试的夹具缺少内容流程必需的 `access_mode=read_only`，该轮失败；修正夹具后按上述最终命令复验通过。首轮结果保留在本聊天工具记录，不作为通过证据。

本次测试不执行真实 Orca/模型、产品 PR 交付流程、安装部署或旧现场改名；不将受控验证视为这些能力的实测。两仓源码 PR 按用户授权另行提交创建。
