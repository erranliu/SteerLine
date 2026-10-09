# Plan：Orca supervised 增加本机目标

本聊天直接实施，无 roadmap Task ID。用户确认以现有 orcaremote 功能为标准，增加本机目标；原本机 Orca 源码保留，切断产品注册、配置 codec 和原 conversation 接线。structured 已推迟，不属于本次。

## 范围与实施

保留 `host=orca_supervised`、`orca_supervised` 协议、当前 supervised 生命周期及远端配置行为。`orca.supervised-profile` 增加 `target=local/environment`，省略 target 仍代表现行 environment 配置；local 不携带远端连接字段，使用项目本机仓库。共享 Run/coordinator/inbox/Task/Dispatch/问答/结算；本机 workspace 操作不加 environment，worker-start 不加 on，本机控制和执行 runtime 相同。远端仍保留独立两 runtime、配对/repo 与 host 校验，失败不会改投本机。

不新增取消、结果文件冻结、交付后自动释放或后台迁移：两 target 保持现有 supervised 行为。消息落库后 ack、原句柄续轮、中心停服只解除观察；本机不能用结构化会话伪造 terminal 身份。原 `host=orca` 与旧 terminal 协议不再被产品 directory 接受，旧源码及 adapter 单测仍保留，配置需显式选新链路。不安装、不修改运行数据库，不重派 105，不把本次修改推入状态修复 PR #265。

1. 配置、预检、workspace/start/show/settlement 接入 local，保持 environment 路由。
2. 删除旧产品注册/codec/原 conversation reader 选择，更新当前配置样例和受影响 fixture；原实现源码保留。旧本机专属 live/startup/candidate 场景从 runner 登记移除，驱动源码保留，不将其静默改成新链路。原远端交付场景保留。
3. 参数化最小 fixture 验证完整生命周期与身份失败边界，定向 app 注册/配置回归；记录[验收](../acceptance/orca-supervised-targets.md)。

## 测试计划

| 验收 ID | 级别/环境/时点 | 命令 | 条件/证据 |
| --- | --- | --- | --- |
| OST-A1 | Go，无模型，步骤 1 | `go test ./internal/execution/adapters/orcaremote -count=1` | 本机配置、预检、路由、原会话续轮、问答/完成/恢复；原远端确定性回归，opt-in live 未选择单列；终端结果与 acceptance |
| OST-A2 | Go，无模型，步骤 2 | `go test ./internal/app -count=1`；`go test ./tests/integration -run '^TestTM06ResourceWiring$' -count=1` | 本机与远端同一 factory，旧 host/codec/protocol 不可达，配置冻结/产品路由不变；acceptance |
| OST-A3 | Go，无模型，步骤 3 | `go test ./internal/testkit/boundary/... ./internal/testkit/protocol ./tests/e2e/internal/native ./tests/e2e/internal/testenv ./cmd/test-feedback -count=1`；`go test ./internal/execution/adapters/orca -count=1`；`go build -o <独立临时目录>/steerline.exe ./cmd/steerline` | adapter 依赖边界、预检 fixture 和当前 runner 入口编译/选择；原模块保留且单测可用；产品构建通过；acceptance |
| OST-A4 | 文档静态，步骤 3 | `python scripts/check_doc_links.py`、`git diff --check` | 配置说明、原路径停用说明和导航正确；acceptance |

不默认跑全 L1/L2/L3/full：本次增加目标路由和禁用旧注册，能力不扩张，以原远端回归和同一 lifecycle 的本机 fixture 覆盖。真实模型提交的 Enter 问题不属于该验证；未选 live 不宣称真实模型稳定性通过。若实际修改测试 runner 或公共产品语义，增加对应最小回归后登记。

原本机产品集成 fixture 保留源码，入口处明确标记 retired/skip；不将其计入当前自动验收。本机 supervised 用新增目标测试与 app factory 接线验证，历史 direct adapter 单测继续运行。
