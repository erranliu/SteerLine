# Spec 工作区与 Git 资源迁移

任务 ID：55；任务名称：工作区与 Git 资源迁移；目标：`EXECUTION-ISOLATION`。状态：已实施，待合入。配套 [Plan](execution-isolation-02-workspace.plan.md)。硬前置：[#54](execution-isolation-01-contracts.spec.md) 已合入，一个实现 PR 交付。

## 目标与范围

建立 `internal/steerline/workspace`，迁移本机 Git/worktree 操作及实际资源解析，使用 #54 的工作区契约。遵守[共同约束](execution-isolation-constraints.md)，保持访问范围、分支命名和清理行为；Orca 工作区创建由 #58 接续迁移。

## 文件与职责

| 文件 | 迁移范围 |
| --- | --- |
| `git.go` | 必要 Git 调用和仓库身份解析，接收固定命令、位置与预算，不读取父包 Config |
| `prepare.go` | worktree 创建/准备、原意图重放及部分资源事实 |
| `inspect.go` | checkout/common-dir、branch、base、HEAD 和固定资源核验 |
| `resources.go` | linked worktree 的 objects、refs、logs、admin 等实际目录解析 |
| `cleanup.go` | 对已经明确持有的精确资源执行清理并返回确认、失败或未知 |

主要输入来自 `workspace.go`、`codex_git_sandbox.go`、`orca_worker_workspace.go` 的共同部分以及现有清理调用。参数只包含固定仓库/worktree 身份和必要工具配置；路径解析验证绝对路径、符号链接/重解析点与真实 Git 身份，不能拿本机路径推定远端资源。

Command/业务持久记录归 contracts，WorkspaceEvidence/ExecutionTarget 等底层值归 execution；操作输入直接用既有 #54 契约或必要转换，不新增完整记录。workspace 可使用纯值，不借它导入中心、query 或 sqlite。远端工作区保留平台 ID 与原路径，不交给本机 Git 根目录解析或清理。

中心的纯工作区命名/base 意图构造继续保留项目决策；本轮迁移的是执行和核验。prepareWorktreeRoot 的路径重叠规则纳入操作输入校验，不能因为拆包把它从领取前移到资源创建之后。本机清理消费 `native_cleanup.go` 的固定意图并返回事实，Task 丢弃资格与业务清理阶段不移入包内。

`resources.go` 返回资源种类及规范路径，不返回 `--add-dir`。现有四组 Git 写目录的授权范围不扩大，尤其不能用整个 Git common-dir 替代指定目录。Codex 参数生成进入 `codexworker/permissions.go` 的现有行为 helper，#57 完成统一命令接线，#59 再改权限规则。

## 中心与副作用

中心保存创建/清理意图、核对业务所有权与资格，再调用工作区操作并保存部分资源和回执。包内不创建 Task/Execution、不 reset 当前代码、不重建丢失现场，不决定任务结束或资源释放时机。创建请求未知时先查询原资源，不另造工作区；清理失败不撤销交付。

保留当前本机根目录重叠限制和分支前缀，不新增分支命名配置。Orca 当前路径转换成共同事实，但其平台 I/O 不混入 workspace 包。当前格式和恢复表示不因搬包改变。

## 并行与交接

可与 [#56](execution-isolation-03-publication.spec.md) 并行。共同 `execution/workspace.go`、主包 Config、准备与清理接线集中集成；#56 在未合入本任务时可以使用已有唯一 Git 执行边界，不能复制工作区解析，最终 #57 接收两者的生产接线。

本任务交付规范资源和校验事实；#59 决定是否授权这些资源。资源发现与权限授予不得混为一次自动扩权。

## 最小验收

| 编号 | 通过条件 |
| --- | --- |
| EW1 | workspace 不依赖父包或业务事务，生产本机准备/检查/清理使用唯一实现 |
| EW2 | 隔离 linked worktree 的原仓库、分支与 base 核验通过；重定向或其他仓库的路径拒绝 |
| EW3 | Git 资源解析保持精确范围，Codex argv 生成与路径解析分开；部分准备/清理回执仍可保存并对账 |

复用真实临时 Git fixture 和现有清理接线，保留一个有效场景及相关拒绝断言，不扩展平台矩阵。
