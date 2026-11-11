# SteerLine × SteerRunner 新建 worktree 启动边界 Spec

适用范围是 SteerLine 提交的 Git 型新 Run，其中任务 101 的开发链路是本次专项验收对象。实施步骤见 [Plan](../plans/fresh-worktree-startup.plan.md)，通过条件和实际结果见[专项验收](../testing/acceptance/fresh-worktree-startup.md)。

## 目标与现状

每次正常启动都由 Runner 在 SteerLine 选定的完整 Git commit 上创建专属 worktree。创建本身保证起点，因此成功路径省去再查 HEAD、分支、diff、洁净度及材料交付的启动门禁。工作区创建结果不确定时，沿[旧 Run 恢复流程](../../docs/product-design/runner-lifecycle.md)处理。

实施前，Runner 仓库准备强制刷新目标分支并重做基线祖先判断，本机 Codex 在创建后重复查询 Git，开发 Workflow 隐式读取项目策略槽位并据此准入。任务 101 已有自动生成的安排，SteerLine 启动仍要求固定 commit 包含 Plan 同名的 `.tests.json`。[Runner #99](https://github.com/erranliu/SteerRunner/pull/99) 已关闭，后续使用普通文件通道。

## FWS-01 · SteerLine 冻结启动条件

SteerLine 负责核对目标分支、任务依赖、环境资格和项目材料，选定已发布、可在目标环境取得的完整 commit。Spec、Plan、适用策略、测试清单、测试规则及同源 writer 等静态引用按该 commit 校验。开发 Workflow 的项目策略槽位保留现有路径和阶段覆盖。可选槽位的缺失与已存在但不可读分别处理，前者沿用默认规则，后者在 SteerLine 明确阻断。SteerLine 随请求显式提供适用策略引用，Runner 仍按 Workflow 阶段路由引用。

候选冻结 Git 基线、执行配置、材料来源和原始字节。「已就绪」和构建启动请求消费同一份冻结材料。启动按原事务确认候选身份并保存完整 outbox，再向 Runner 投递。预检失败时返回具体缺项。已提交请求的重读或对账沿原请求及原身份进行。

## FWS-02 · Runner 创建工作区后进入 Workflow

Runner 获取请求指定的 commit，使用该对象创建专属 worktree，并将实际创建回执绑定到原仓库、目标和 Run。对象无法取得、创建失败、路径冲突、回执身份不符或操作结果未知均按实际情况处理。后续目标分支推进或改写，只要固定对象仍能取得，就不会改变本次创建起点。创建成功的 Git 事实来自原创建操作或 Orca 创建回执，观察口径仅记录有来源支持的事实。

本机 Codex、本机 Orca 和已支持的远端 Orca 都遵循此启动规则。Orca 的 host/worktree/base/head 回执字段仍用于确认原创建结果。Runner 继续管理路径、权限、资源及请求结构。运行期文件传输沿用原通道和诊断语义，文件的业务资格及实际送达无需 Runner 另行确认或据此决定 Workflow 能否启动。

## FWS-03 · 任务 101 的安排由候选供给

SteerLine 用固定 Git 清单核对自动生成的有序测试 ID，将既有 `{tests:[ID]}` 格式的原始字节与摘要随候选冻结。启动用同一份字节，通过已有普通 `result_file.initial_content_base64` 交给 Runner 的工作区文件通道。提交前核对目标组合支持该通道、请求形状正确且解码后不超过 64 KiB。进度仍走独立的 `progress_file`。

短指令只给出正式读取定位、摘要及用途。Agent 在执行依赖安排的动作前读取该文件并确认它对应本次冻结安排。读不到或摘要不符时，由 Agent 报告阻塞。原始安排不进入提示词，不向业务仓库写入或要求提交旁置 JSON，也不以空安排继续。SteerLine 仍按原项目验收口径消费进度和交付事实，result 文件本身不构成测试验收。此条取代[调用方提示词 Spec](caller-prompt-convergence.spec.md)中对本路径同名 `.tests.json` 的要求。

## FWS-04 · Workflow 自治及旧 Run 恢复

Runner 按选定 Workflow 自行处理实施、审计、修复、发布与既有阶段转移。Workflow 内的 Git 事实、输出判定和发布核对继续用于流程推进。SteerLine 通过公开观察显示进度，负责项目验收和交付。

旧 Run、创建结果未知和现有 worktree 的使用属于独立流程。本次保留其原资源、Git 检查、对账及停止语义，禁止把已存在 worktree 判成新建成功或补发一次创建。独立内容型 `generate`、`conversation` 和 Roadmap 材料契约、项目交付接口以及未支持的执行组合均在本专项之外。

## 通过条件

- 任务 101 在无旁置 JSON 的固定 SteerlineTest 基线上进入「已就绪」，并通过正式启动入口把相同字节交给 Agent 读取。无通道、超限或静态材料错误时，SteerLine 在提交前阻断。
- 可证实创建成功的新 worktree 从固定 commit 启动，即使目标分支已变化。成功路径无额外 HEAD、diff、status 门禁，缺对象、假回执及创建未知均不能进入此成功路径。
- 注入安排传输失败或内容不符时，Runner 仍可派发步骤，Agent 在依赖安排的动作前报告阻塞。进度、原请求对账和旧 Run 恢复有回归证据。
- 普通及审计 Workflow 的适用路径仍自主推进。受控测试与本机、Orca 原生资格分别记录。
