# L1–L4 测试内容调整验收

日期：2026-11-02。普通开发，依据 [Spec](../../spec/test-content-alignment.spec.md) 与 [Plan](../../plans/test-content-alignment.plan.md)。用户授权先文档再实施，独立审核后提 PR。

PR #423 创建后，用户追加要求修复 Go Full 的全部失败且不放宽断言；续修结果见[专属记录](go-full-repair.md)。本文保留初轮运行与当时结论。

| ID | 范围 | 状态 |
| --- | --- | --- |
| TCA-A1 | 正式本地回归、失败及完整性 | 执行器完整回归及新反例通过 |
| TCA-A2 | 最终固定 L1/L2 | 通过，L2 清理完整 |
| TCA-A3 | Web/owner/Full/Runner 边界 | Web/HTTP/存储/环境/空闲契约通过；Full 已执行但失败，详见下方，不计通过 |
| TCA-A4 | 材料生成与配置 L3 组合 | 第四轮两项均通过，实际重叠74.084s，各自清理完整 |
| TCA-A5 | 文档、独立审核、PR | 文档/实现最终独立审核通过，准许按明确未验/失败边界提 PR |

## 调查基线

源码 `6f1edde1dbc9bb950d579a4cbe1dca67301c778b`，干净。原 L1 1、L2 1、L3 8、L4 14。L1 passed，准备0.462s/执行26.207s；L2 full-L2 passed，准备14.669s/执行111.787s，清理完整。原始目录分别为 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261102T022813.819220700Z` 与 `20261102T022906.192523000Z`；不算调整后验收。

## 提示词及审核

独立文档审核发现 CI 缺前端依赖准备及“三份材料自动生成”表述错误，均在实施中修正。独立实现初审发现 GOFLAGS 静默缩窄、默认空夹具、managed 写失败提前发布完成三项问题；均已修正并经独立复核。后续配置准备与自有分支修正另经独立审核通过；本结论不代替尚未完成场景的实际结果。

| 提示词相关范围 | Git 规范化正文 UTF-8 字节前→后 | 实际正文差异 |
| --- | --- | --- |
| 固定 Markdown 10 文件 | 7,279→7,279 | 无 |
| 黄金样本 7 文件 | 2,588→2,588 | 无 |
| 正式 writer | 3,717→3,717 | 无 |
| 提示词/材料/组装/样本/边界与 writer 共 23 文件 | 30,682→30,682 | 无，git diff 为空 |

保留用户 CRLF 的黄金样本逐字节核对，其他文件按 Git 正文核对；既有行尾物理长度差不算本次正文变化。CI 实际只增加 setup-node@v7、Node 24、npm 缓存及 npm ci 八行；业务原因是固定 L1 新增复用的前端最小测试，需要干净 CI 提供依赖。原提示词 owner 测试列表、正文断言、源码守卫、job 名、只读权限和触发范围全部保留。独立审核明确批准该 CI/选择/报告变化，没有更新任何黄金样本。setup-node 用法对照[官方说明](https://github.com/actions/setup-node)。

## 运行与边界

实施后逐项追加命令、源码/dirty、输出目录、结果/清理和审核。真实模型、远端及 GitHub 测试 PR 不执行；重型浏览器 L4 不改级。最终交付为本仓代码 PR。

### 实际执行

以下源码为 `6f1edde1dbc9bb950d579a4cbe1dca67301c778b` 加本 PR 工作树差异，dirty=true；不导出为干净 commit 的 Git 历史。正式报告各自保存实际 before/after、定义及命令。输出目录前缀为 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/`。

| 正式选择 / 命令 | 结果 | 原始输出叶目录 |
| --- | --- | --- |
| `-level=L1`，含前端最小集 | passed/complete，准备0.293s、执行16.133s，120个测试终态、0 skip | `20261102T032152.568089800Z` |
| `scripts/test-l2.ps1` | passed/full-L2，准备7.487s、执行91.925s，cleanup_complete=true | `20261102T032158.575689500Z` |
| `-level=L4 -suite=web -scenario web.unit` | 31文件、228断言、0 skip，passed，23.383s | `20261102T025000.015475900Z` |
| `-level=L4 -suite=go -scenario go.http` | 3包、104终态、0 skip，passed，59.294s | `20261102T024959.880378300Z` |
| `-level=L4 -suite=go -scenario go.storage` | 2包、82终态、0 skip，passed，98.479s | `20261102T024959.899517000Z` |
| `-level=L4 -suite=go -scenario go.environment` | 4包、328终态、0 skip，passed，1177.608s（包含完整 app owner） | `20261102T024959.928671600Z` |
| `-level=L4 -suite=go -scenario go.runner-contract` | 2包3测试、0 skip，passed，52.445s；自有空闲服务停止/结算断言通过 | `20261102T025213.624141100Z` |
| `-- ./cmd/test-feedback -count=1` | 完整执行器 owner passed，40.178s；GOFLAGS 缩窄和真实报告写入失败/进度反例实际命中 | `20261102T025722.911699600Z` |
| 最终期限/清单三项定向 | passed，7.282s；配置父级覆盖既有10分钟业务窗口及独立清理，未延长 body | `20261102T030633.424161700Z` |
| `-level=L4 -suite=go` | failed/退出1/managed failed；准备15.2s、执行1297.838s；71包含14个编译项，1427测试终态、0实际测试skip；3包13失败终态（含子项） | `20261102T025454.875678400Z` |

独立 Runner 使用 `C:/Project/SteerRunner/bin/steerrunner.exe`，公开 version `0.2.0-dev.4`、source `e2bc0a578047fa38293edbe411b15a386ddd20a4`、SHA256 `b7b3ccf106c4879a9d3998d0cacd99f662885530e581443981615c7478967be8`。只运行空闲契约，不调用模型/发布。`go.runner-result` 缺独立原生 fixture，本次未运行，不计通过；缺输入阻断已由执行器反例验证。

`npm --prefix web ci` 与串行 Web 构建通过；全部 E2E `-tags=e2e -run '^$'` 编译通过（不计场景通过），test-web.ps1 AST 通过，Markdown 640文件/4,012本地链接、0错误，diff 检查通过。

改动后的 `scripts/test-web.ps1` 另实际完整通过，`C:/Users/lmas1/AppData/Local/Temp/steerline-web-check-20261102-111613-933/` 保存 build.log、client-tests.log、server-tests.log 与正式 web-unit/go-http 子报告，验证参数及报告路径确实接通。

第四轮组合根为 `C:/Users/lmas1/AppData/Local/Temp/steerline-tca-combination-20261102-04/L3/`。准备8.838s、执行367.113s，package_workers=2；配置/材料包均于 `2026-11-02T03:08:27.0085967Z` 开始，分别于 `03:14:33.7568846Z`、`03:09:41.0928992Z` 结束，实际重叠74.084s。两项 scenario 的 passed/cleanup_complete 均 true，0 skip。配置实际366.748s，未延长既有10分钟业务窗口，原6分钟父级确会裁剪该有效执行，因此对齐为12分钟。只证明此组合，不泛化全部组合或真实 Orca。

### 原失败与修正

- 首次 Web 入口因依赖未安装明确失败，保留 `20261102T024719.924224600Z`；安装 package-lock 固定依赖后重新完整执行通过，不跳过前端。
- 实际 PowerShell 将未引用的 `-scenario=web.unit` 拆成两个参数，入口拒绝，未执行测试。清单/文档/脚本统一采用 `-scenario web.unit`；正式命令选择测试全部通过。
- L3 组合原始根目录为 `C:/Users/lmas1/AppData/Local/Temp/steerline-tca-combination-20261102-01` 至 `-04`，各轮报告在 `L3/`。首轮材料安排为空及旧配置缺环境准入，两个场景失败；仅新增场景改用自有非空定义，配置通过公开入口准备环境，共享 L2 夹具不变。
- 第二/三轮材料生成通过；配置仍因候选分支上的新环境材料未进入 main 而违反 source_commit 断言。两轮 cleanup_complete=true，原失败不改写。定位公开 PR fixture 会 checkout 候选分支；修正为回场景自有 main 后提交/push、公开维护刷新、全部 Agent 环境分别准备完再启动候选。原来源/停止/候选断言保留；原10分钟业务期限不变，只修正原6分钟 Go 父级为12分钟，避免裁剪既有窗口及清理。

### Full 失败归属与独立复核

普通 Full 没有条件测试跳过，14个无测试文件包均在 packages_without_tests 明确枚举；真实失败正确使原 summary 不通过、进程退出1、managed-go.full 为 failed。未删除失败项或放宽测试定义。

| 失败包 | 原失败 / 基线核实 |
| --- | --- |
| `internal/testkit/boundary` | TestCallerOwnerBoundary、TestTM04ExecutionBoundary（含 repository 子项）、TestHostCheckpointOwnership、TestTaskDeliveryOwnership、TestThreeLayerRepositoryBoundary，共6失败终态；干净6f1基线全部同名复现，涉及已有模型/提示词/Runner 的所有权及依赖矩阵，不在本轮修改生产依赖或放宽规则。 |
| `web/server/executionconfig` | TestRunUsageConfigPreservation、TestGQConcurrencySaveAndValidation、TestExecutionConfigSave（含 canceled_before_publication）、TestExecutionConfigConcurrentAndFrozen、TestExecutionConfigSchemaAndPreservation，共6失败终态；干净6f1的三次定向全部复现，原 profile/capabilities schema 夹具不一致。 |
| `internal/product/httpfs` | TestPrivateSnapshotConcurrentPublication 一次失败：并发读 receipt.json 时 GetFileAttributesEx 返回不存在；原测试及实现未变。干净6f1一次加三次定向均通过，原因尚未定位，不称已修复或确定为基线故障。 |

基线使用 `git archive 6f1edde1dbc9bb950d579a4cbe1dca67301c778b` 导出到全新 `C:/Users/lmas1/AppData/Local/Temp/steerline-tca-baseline-20261102/source/`，不切换当前 worktree。相同失败名称的定向命令仅选择 httpfs/boundary，报告在同根 `reports/`；后续 httpfs 与 executionconfig 定向 `-count=3` 在 `remaining-failures/`。两份 summary/原 JSONL 保留，基线无 .git，不冒充有干净 metadata 的 managed 执行。独立审核另核对上述生产源码/测试/能力 schema 的 Git 正文对象均未变，Full 原 other-packages 并发/隔离选项也沿用原实现。

最终独立审核结论：三个初审发现均修复，配置准备/分支及期限修正合理，提示词门禁保留，未发现本轮新增实质问题，可以提 PR。明确 Full failed、基线复现范围和 httpfs 未定位，不以其他集合通过替代 Full；`go.runner-result` 仍未实际运行。没有新增人工验收、真实模型/远端/测试 PR、安装变更或重型场景改级。

最终代码在独立审核后再次完整运行固定 L1/L2，结果见表；最终全部 E2E 标签编译也通过。之后只更新本验收记录与提交元数据。实现中此前的 L1/L2 通过报告仍保存在 `20261102T025922.149047200Z`、`20261102T025711.834363400Z`，不覆盖或冒充最终运行。当前正式清单为31项：L1 1、L2 1、L3 9、L4 20。
