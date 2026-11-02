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
