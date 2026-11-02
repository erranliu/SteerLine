# Plan：L1–L4 测试内容与执行口径对齐

依据 [Spec](../spec/test-content-alignment.spec.md)，普通开发直接实施；运行/失败/审核见[专属验收](../testing/acceptance/test-content-alignment.md)，路径遵循[清单](../paths.md)。

## 实施步骤

1. 先建立 Spec/Plan/验收及导航，独立只读审核范围与可验收性，修正文档确认问题。
2. 提取正式本地回归定义：固定 ID/suite/环境及包/文件范围，供解析、执行、清单校验及 managed 冻结共用；不改变 L2/E2E 清理和业务断言。
3. 外部 Runner 测试加专属标签，保留全部断言。Go Full 拒绝实际测试 skip（无测试文件包另枚举为编译项），以同一完整性事实驱动 summary/退出码/受管结果。owner 集核对实际包、非零用例及终态，不接受任意 Go 参数伪装正式集。
4. 接入 Vitest 原生 JSON，核对所选文件、全部断言终态、零跳过及非零匹配，保留原始输出；L1 复用两份既有文件，web.unit 执行全集。Web 失败不被 Go 成功覆盖；现有 CI 增加 Node 与 npm ci，CI 差异纳入提示词治理独立审核。
5. 新增公开材料生成 L3，验证 ready/版本/一键空批次及无业务候选；不扩张 Runner 模拟实现。同步机器清单、manifest、各级列表、指南、报告/路径和脚本。
6. 运行必要检查，独立复审完整差异，修复确认问题并复验；提交、推送、创建 PR，不合并或部署。

## 测试安排

| 验收 | ID / 范围 | 命令、时点与通过条件 |
| --- | --- | --- |
| A1 | 执行器 owner | 实施后 `go run ./cmd/test-feedback -- ./cmd/test-feedback -count=1`；合法/非法选择、缺失/skip/零匹配/失败与原始/受管结果一致。 |
| A2 | l1.minimum、smoke.delivery | 最终实现后正式 L1、独立同用户 `scripts/test-l2.ps1 -OutDir <新目录>`；零跳过、passed/complete，L2 cleanup_complete。 |
| A3 | web.unit、go.http/storage/environment/full | 正式 `-level=L4 -suite=web/go -scenario <ID> -out <新目录>`；完整包/文件和终态，无条件 skip；Go Full 其他失败分别定位，不以子集代替。 |
| A3 | go.runner-contract/result | 缺输入反例由 A1；已有固定 Runner 可用则运行空闲契约。结果专项缺 fixture 明确未验，不启动模型代替。 |
| A4 | 材料生成 + 配置 L3 | 先串行 Web 构建，再 `-level=L3 -suite=testing,configuration -env=controlled -scenario testing.material-generation,configuration.execution-profiles`；两项 passed/cleanup_complete，包间时间重叠。 |
| A5 | 文档、标签编译、审核 | `python scripts/check_doc_links.py`、`go test -tags=e2e ./tests/e2e/... -run '^$'`、`git diff --check`；清单一致，编译不算场景通过；独立审核覆盖提示词 0 字节变化。 |

原始输出使用独占系统临时目录或显式外部 scope；每次补测新目录，失败不覆盖。首次完整检查后只复验新修改/未决问题；不自动重试。无新增人工验收，不执行真实模型/远端/测试 PR 或重型场景改级。

## PR #423 续修安排

依据 Spec 续修授权，先修复三类失败再重跑全套，不修改边界矩阵或减少原断言：

1. 将 ProjectEntry 及原测试移到测试模型 owner，更新所有引用；应用向 GenerationSources 显式提供权威协议正文，缺失仍阻断。runnerstate 独立声明原四字段准入证据，应用/SQLite 转换核对；环境 model 的外部测试改同包，保留断言。
2. Windows 私有快照先取得共享删除/替换句柄，再对同一句柄核验类型和完整私有 ACL；SID 级私有内核互斥体仅协调打开/发布，覆盖不同进程及路径别名，不在读取期间持锁。Unix 防符号链接、FIFO 非阻塞拒绝及已解绑 inode/多个硬链接边界分别回归，路径级校验不变。增加已打开文件与路径替换的确定性回归，原并发1000次发布断言保持。
3. 正向配置夹具补 access_mode；保留可用 executable，并将已不被公开 run schema 接受的 github_auth 改为显式负例。可选字段保留用现行合法字段验证，保留原标记断言并增加完整 settings 字节相等断言；不放宽 schema。定向复验执行配置、httpfs、完整 boundary、projectmaintenance/testmanagement/prompts/SQLite 等 owner。
4. 执行最终 L1/L2、受影响材料生成/环境准备 L3 及普通 go.full。Full 完整通过后，只因新修改/失败才重跑；原生 Runner/真实模型/安装及远端仍不执行。独立审核全部差异与提示词黄金正文、字节数，更新 [修复验收](../testing/acceptance/go-full-repair.md)，提交/推送同分支并更新 PR 描述。

### 主干冲突续修

用户追加要求解决 PR #423 与 main 的冲突。合入 `fea9df06`，保留主干中性目标/环境分组、公开双项目准备和新增测试，同时保留本 PR 四字段 DTO、强断言和命令修正。实际边界回归发现主干新增 model→executionconfig 不合法依赖；模型仅校验自己的输入形状，跨 owner 的目标表示断言移到 App 完整保留，中性 Target 仍归 executionconfig，扫描矩阵不改。独立审核冲突及补正；运行 L1/L2、边界/清单/配置及模型 owner、Web 全集、App 准入/分组和共享 Runner L3，原 Full 按原源码保留为历史，不冒称合并后全量通过。记录新失败与修正后结果再提交推送。
