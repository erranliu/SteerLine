# Codex 沙箱开发与真实 Git 发布修复 Plan

普通开发，基线 `fbde4163`；验收归属：[Codex 沙箱 Git](../acceptance/codex-sandbox-git.md)。保留 workspace-write、unelevated、on-request、auto_review，worker 负责提交和发布；不改 DAG、上层状态机或安装环境。

## 实施步骤

1. Codex adapter 为 worker 选择真实 Windows PowerShell，避免 WindowsApps 执行别名；禁用 login shell，并在原生线程内提供固定工具和权限故障处理指导。
2. 当前 linked-worktree 的已授权 admin 目录内建立专属临时目录，设置 TEMP/TMP，校验冻结的 Git 根和路径链接；Git 自动维护仅在 worker 进程环境中关闭，保留已有进程级 Git 配置与认证。
3. 认证 helper/系统对象启动被拒绝时，worker 使用原生自动审核申请固定命令重试；拒绝或不可用不得绕过。发布重试先查询真实分支/PR，不用创建远端 ref 的 API 代替上传 Git 对象。
4. 更新 debug-template 的真实任务探针和可重试发布脚本，显式使用 gh shell credential helper；新增 opt-in 真实 Codex 驱动，自动创建两个候选、发布并核验真实 PR，第一项合并到本包专属 base 后再启动依赖任务。只清理本次专属资源；不使用 C:\SteerlineRuntime，不写用户全局 Git/Codex 配置。

## 测试计划

全部证据保留在新生成包的 evidence/，结果登记在专属验收文档。已有 debug 入口 CRLF 和私有 ACL 修复结果另见其原验收。

| 验收项 | 级别/场景 | 环境与命令 | 时点、通过条件、证据 |
| --- | --- | --- | --- |
| CSG-01、CSG-02 | 定向规则/真实临时 Git | Windows；`go test ./internal/execution/adapters/codex -count=1` | 实现后；实际 exe/alias 区分、temp 在固定 admin 内、链接拒绝、Git env 合并、不扩大写根；保存 targeted.log |
| CSG-03、CSG-04、CSG-05 | 独立包真实 Codex/GitHub | Windows live；`powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/test-codex-sandbox.ps1` | 定向通过后；两项原生三步轮次、PowerShell/temp/项目检查、精确 commit/push/PR、自动审批事实、依赖交付、兄弟写入拒绝；保存 native journals 和 sandbox-live.json，不通过 Ctrl+N 或手工批准 |
| 回归 | 独立调试 controlled | Windows；`powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/test-debug-release.ps1` | live 前；模拟链/重置/隔离通过；generation-test.json 和 runtime-test.log |
| 回归 | L1 最小固定组 | Windows；`go run ./cmd/test-feedback -level=L1` | 实现后；配置/资源冻结、事务及接线固定组通过，runner 输出目录记录 |

新增真实驱动及 fixture 探针在实施中补齐，不以模拟或无模型脚本 PR 代替真实沙箱证据。无需 full、全部 L2/L3 或其他 agent 实测；此改动仅面向本机 Codex adapter，Orca supervised 不套用本机环境修复。人工项：原生查看界面的体验可后置，不阻塞本次无人值守验收。
