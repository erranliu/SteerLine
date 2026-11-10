# 核验成本精简验收

日期：2026-11-10。SteerLine 基线 `4f1a4bcb`，验证对象为本工作树未提交改动；Runner 基线 `5ccc3f0a`，修改仅位于独立 worktree，分支 `codex/reduce-verification-overhead`。本次不部署、不变更远端保护或历史 Run。

## 集成主干前的原始改动与提示词审核

- `internal/prompts/materials/testing.md` 及正式项目副本：删除普通执行前逐 ID 调查并补改接线的要求，将接线和契约验证归入初始化、新增 ID 或入口变更。普通任务仍执行固定安排，异常保留未完成进度与原报告。
- 实际正文替换：`执行前逐项核对固定安排中每个测试 ID 与其真实入口的进度接线`及其补接线要求，替换为`普通任务直接运行已登记入口，不逐 ID 调查或补改进度接线；项目初始化或测试入口变更时完成接线与契约验证`；writer 段的逐项调查要求同步改为入口维护职责。
- 规范化 UTF-8 正文由 **3256 B → 2951 B**，减少 305 B。短提示模板和调用方黄金正文不变；长规则摘要由 `75671b11e26c540791678c1593c3b14ed4161ba53b721d8758ae8c9c1eba937a` 改为 `1cb27f037d486d2445921bec578020311229332fe24996ac4655e44610261e82`。完整实际差异以这两个 Markdown 的 Git diff 为准，摘要更新不代替审核。
- 业务原因：把测试基础设施维护从每次实施/审计中移出，继续由真实入口及其契约测试证明接线。同步初始化指南、测试指南和进度接入说明，并修正初始化指南中与现有固定文件供给不符的 inline 描述。
- `Selection.Open` 先做无文件读取的绝对路径/必填摘要格式准入，由 `Version → Client.Call` 执行调用前哈希，返回后再次复核；成功路径全量哈希 **3 → 2 次**。每次实际 CLI 调用前的 EXE 哈希继续保留，不引入长效缓存。

## 必须保留的边界

冻结安排、同源固定协议和 writer、真实非零选择、失败/跳过/清理要求、原报告留存、禁止模型事后填 true 均保留。PR 发布核验与任务合并交付核验仍分属 Runner/SteerLine；交付事务重核、未知请求沿原 ID 对账及停止结算不变。

## 独立审核

只读审核者 `/root/review_steerline` 审阅完整正文、正式副本、摘要、文档和实际客户端调用路径。首轮发现 P1：空摘要可能跳过 `Client.Call` 的条件核验，导致 EXE 先运行。已增加无 I/O 的 pin 格式准入和空/非法/短摘要零 CLI 调用回归；复审明确批准，P1 关闭，未发现其他阻断问题。

Runner 由另一只读审核者 `/root/review_runner` 独立检查，未发现 P0–P2；详细记录见其 worktree 的 `docs/acceptance/verification-overhead-reduction.md`。

## 验证

统一证据根：`C:\SteerlineDebug\wt-5d04-verification\`。输出独立，不覆盖失败现场。首次沙箱内 Runner owner 测试因临时 Git config/文件替换权限失败，npm ci 因 exit handler 异常退出；未计通过。之后通过正常权限在隔离目录重新执行，锁定前端依赖安装成功。

| 验证 | 结果与原始定位 |
| --- | --- |
| `go test ./internal/prompts ./internal/runnerclient ./internal/testmanagement ./cmd/test-feedback -count=1 -json` | 431 个测试及子测试通过，无失败/skip；`diagnostics/steerline-owner-aa3e014caa924402a2cec7b87c65a0b9/go-test.jsonl`。此轮早于 pin 审核修复，最终客户端另验如下。 |
| `go test ./internal/runnerclient -count=1 -json` | 最终 pin 修复后 137 个测试及子测试通过，无失败/skip；`diagnostics/release-pin-2d9a52c62de94a19baba89e0a3c5fef2/go-test.jsonl`。 |
| `go run ./cmd/test-feedback -level=L1 -out <独立目录>` | 通过，准备 0.2s、执行 20.1s；`test-results/L1/20261110T073219102Z-0b494dd588034bb197b038d592b7d04a/summary.json`。 |
| `go run ./cmd/test-feedback -level=L2 -out <独立目录>` | 通过；`test-results/L2/20261110T073320698Z-f2d7b2652c8246e2b0787531147383ff/summary.json`。 |
| `python scripts/check_doc_links.py`、`git diff --check` | 最终检查通过；737 个 Markdown、4562 个本地导航链接，0 错误。 |

尚未进行真实模型/真实 PR、安装部署或端到端耗时比较；19→14 和 3→2 是调用次数变化，不声称相同比例的总耗时收益。测试历史不以脏工作树结果导出为干净提交通过。

## Runner 最终结果

Runner owner 29 项通过；正式短根 L1 通过（758 个顶层 Go 用例及框架/原生驱动检查），报告 `runner-l1-short-aca5d5d25e2549c580623bb48f28ca10/stages.json`。首次较深临时根 L1 的长分支路径用例失败，在短根保留同一断言后定向及完整 L1 均通过，原失败保留。

受控 workflow/recovery 原组共 12 个顶层场景，11 通过、1 个 NativeUnknownAndStop 回执/停止未核实，组状态保持 failed。原现场 `C:/Users/lmas1/AppData/Local/Temp/sr-54729125` 保留，根因未定。同一冻结 Runner/测试/夹具的新现场定向补验通过，原生输入仅一次，停止、授权撤销及资源结算均核实；不将补验冒充原组通过。独立只读审核已核对这些终态，完整定位和未验范围见 Runner 专属验收。

后续根因排查已通过原 revision 7 事件及基线对照定位：owner-ready.json 在发送前遭遇 Windows 文件共享冲突，未发送操作进入 unknown 后仅查询不存在的原回执；基线5ccc3f0a亦可复现。Runner worktree 已加入仅针对该读取的有界重试，通过独立审核、owner/不可变操作组件回归和3个定向受控场景（含4个重启窗口）。详见 Runner 的 docs/acceptance/owner-read-sharing-receipt.md；原组失败和原停止未知现场不变。

## PR 集成范围

创建 PR 前已刷新主干到 `c8feff352d4cbd19c93f6f2f89d2fd283e1c115e`。PR #491 已交付普通任务不逐 ID 重查接线、按触发条件检查接线及相应规则收敛，因此本分支完整采用主干的测试指南、初始化/进度接入文档、正式 testing 规则和摘要，不重复修改它们。上文 3256→2951 B、正文替换与独立审核描述保留为原实现的历史记录，不宣称它们是本 PR 相对最新主干的正文差异。原实现提交为 `9ceeb5c2`。

最终相对最新主干的交付范围为 `internal/runnerclient/selection.go`、`selection_test.go` 及本验收记录：避免首次打开重复哈希，同时保留 pin 格式准入、调用前实际字节核验及返回后复核。提示词实际正文/黄金样本/摘要相对目标主干均无差异，新增提示词字节为 0。原客户端验证可按来源与范围复用，集成后补跑提示词与客户端定向检查；远端 CI 另按本 PR 最终 head 判断。

集成补验已通过：`go test ./internal/prompts ./internal/runnerclient -count=1 -json`，原报告为 `C:/SteerlineDebug/wt-5d04-verification/diagnostics/pr-integration-90bdef97064a4c0c8ccf263c643ce637/go-test.jsonl`，测试源码版本为 `a63c90b5`。独立审核者 `/root/review_steerline` 复审相对最新主干的三文件差异并批准；确认 pin 保护保留、正式提示词和六份规范文件与主干一致。此后仅补充本段结果，代码未变。
