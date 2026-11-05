# 项目共用环境材料验收

2026-11-02。[Spec](../../spec/shared-environment-materials.spec.md) · [Plan](../../plans/shared-environment-materials.plan.md)。

## 范围与源码

Spec/Plan 独立审计通过后实施。基线 `cf9346759a086d8c242d0b09c480fe9842a3e2db`，测试基于此基线加本工作区修改（dirty），不声称为已提交产品资格。证据 scope：`C:/SteerlineDebug/wt-76f8-shared-environment/`。本次不部署、不修改其他受管项目或安装现场，不启动 roadmap。

公共文档为 SteerlineFlow/environment.md，选定脚本为 SteerlineFlow/scripts/verify-environment.ps1；manifest5 去掉环境键。原 Key/Context/Current/revision、report3 信封、生命周期和候选准入保持。项目脚本只查询 Git/Go/Node/npm/Python 版本，不检测链路、测试、构建或发布。

## 独立审计

独立只读子代理 audit_spec_plan 审计 Spec/Plan 后明确批准进入实施，无阻塞问题；落实其提醒，同步任务测试 ProjectEntry 对新公共环境文档及两个固定入口的排除。

实现首轮复审指出 PS5.1 File.Replace 的 null 绑定问题；已使用 NullString.Value 修复，真实 powershell.exe 正负用例和实际工具脚本执行通过。另保留固定的缺失/低版本/异常/超时原因，并将超时清理进程也限时。最终代码复审已明确批准修正后的实现及提示词/接线差异，新增脚本和测试文件与审核快照 SHA256 一致。L2 随后发现的三个 E2E 驱动公共路径补丁也已独立复审批准；未扩大真实链权限或改变流程。测试现场合并也已独立只读复审批准：固定提交读取减少重复fetch，所有隔离、变更撤销和无回退断言保留。正式测试结果已核实：L1、L2、web.unit 和 go.environment 全部通过，go.environment 零跳过/零断言失败且原预算未变。

## 测试与失败留存

原命令按 Plan，首次定向范围优化为新公共材料/脚本及 manifest、提示词、任务入口 owner 用例（通过 -run 明确选择，不宣称完整 owner）。完整 owner 随 go.environment 正式执行。

| 范围 | 输出（相对 scope） | 结果 |
| --- | --- | --- |
| 首轮定向 | diagnostics/20261102T103841130Z-a8ae697a9895490c9a854704d6aa12ee | 失败：PS5.1 File.Replace 的 null 绑定；公共材料/跨链路隔离用例通过 |
| 第二轮定向 | diagnostics/20261102T103945632Z-be15188f1cf646e7aa9b14c888038bf9 | 失败：脚本写回成功，测试自身 JSON 数字 int/float64 比较不一致；公共材料/撤销用例通过 |
| PS5.1 脚本修正后补验 | diagnostics/20261102T104109185Z-a11ff8cc5c06416cadc7763838bfdbab | passed，6 类输入，身份与原子写回保持 |
| 本机实际工具查询 | diagnostics/script-native-20261102T104240291Z-f5152e5b55be409ca14016837271a49f | 退出0，ready=true；Git 2.46.0、Go 1.26.4、Node 24.13.0、npm 11.6.2、Python 3.13.1；仅脚本结果，不授予真实 Runner 资格 |
| 首轮正式 web.unit | test-results/L4/20261102T104501022Z-7a42e470631948ab810cbf446131271b | 前置失败：缺 web/node_modules，未执行断言；按锁文件 npm ci 后补验 |
| 正式 web.unit 补验 | test-results/L4/20261102T104617162Z-e4d28658201546598c265355d6965a45 | passed=true、complete=true，managed-web.unit 为 passed |
| 正式 L1 | test-results/L1/20261102T104640648Z-49a987a704744ff294ad74f647e0b047 | passed=true、complete=true，managed-l1.minimum 为 passed |
| 首轮正式 L2 | test-results/L2/20261102T104701461Z-2a9c9e464c6148359d622ee9cc0e69be | 编译失败：tests/e2e/internal/testenv 三个驱动遗漏旧路径 API；已改公共路径与 manifest5，不降低断言 |
| 正式 L2 补验 | test-results/L2/20261102T104806678Z-8c80456d0bef454c81d3fe861e2a4a4c | passed=true，smoke.delivery 通过且 cleanup_complete=true |
| 首轮正式 go.environment | test-results/L4/20261102T104147343Z-ef12f6bfc7234d1bbe08bccc34fa7f84 | app 包达到既有25分钟上限；已完成323项无断言失败，新增功能用例均已通过；原日志与未终态项目保留 |
| 合并新增回归现场后的定向补验 | diagnostics/20261102T111001147Z-187ca42f3a8a418ca92d78f66c29ce5e | passed，52.241秒；六次重复fixture合为一个真实Git/SQLite场景，保留全部断言和独立activity/CAS，未改产品或超时 |
| 首轮未终态的尾部用例补验 | diagnostics/tail-20261102T111417471Z-e0d72d7ec0124f64970e930476165c42 | passed，15个根测试及子项共42项，124.700秒；不替代正式全集通过 |
| 正式 go.environment 补验 | test-results/L4/20261102T111125161Z-16d7c9b0604641f580f20a1fc016b480 | passed=true、complete=true，managed-go.environment 为 passed；360项测试及子项通过，零跳过/零失败；原25分钟包预算/30分钟外层预算不变 |
| 提示词 owner / 文档检查 | go test ./internal/prompts -count=1 -v；python scripts/check_doc_links.py；git diff --check | 通过；渲染正文396字节；655份Markdown、4083个本地链接零错误 |

首次沙箱内测试在创建仓库外 scope 时 Access denied，未开始测试；通过本次授权的提权命令写入项目规范证据目录后执行。不是产品测试失败，也不是自动审核拒绝。

## 正文与字节审核账本

首轮完整 Git 正文/接线差异、UTF-8 字节和 SHA，以及新增脚本/测试快照位于 diagnostics/prompt-review/。最终含 E2E 补丁、合并后的测试现场和完整 Spec/Plan 的差异/账本位于 diagnostics/final-verified/（full.diff、bytes.json）；初轮快照不冒称最终测试文件内容。业务原因是取消每环境键的材料复制，新增目标无需 Git 改动。

| 文件 | 原字节 | 新字节 |
| --- | ---: | ---: |
| internal/prompts/environment-invoke.md | 412 | 412 |
| internal/prompts/testdata/environment-invoke.golden.md | 466 | 396 |
| internal/prompts/prompts_test.go | 5819 | 5786 |
| internal/app/environment_adaptation.go | 9434 | 9256 |
| internal/app/environment_materials.go | 1169 | 1161 |
| internal/app/environment_readiness.go | 7269 | 7256 |
| SteerlineProjectGuideDoc/environment-adaptation.md | 9180 | 9460 |
| SteerlineFlow/environment.md | 427 | 2066 |
| SteerlineFlow/environment-common.md | 664 | 0（合并删除） |
| SteerlineFlow/scripts/verify-environment.ps1 | 0 | 5563 |
| SteerlineFlow/testing/test-list.json | 22227 | 22227 |
| tests/e2e/internal/testenv/controlled_environment.go | 6450 | 6260 |
| tests/e2e/internal/testenv/project_progress_live.go | 20607 | 20573 |
| tests/e2e/internal/testenv/remote_environment.go | 16576 | 16530 |

实际完整调用正文唯一差异：`SteerlineFlow/environments/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/verify-environment.ps1` → `SteerlineFlow/scripts/verify-environment.ps1`，466→396字节。其余固定规则完全相同；模板本身412字节不变。未把行为规则追加到业务 Go、schema 或 inline 材料。测试清单两处 manifest4→manifest5 等长替换；接口指导同时去掉逐环境材料要求并明确不检查链路，完整原文 diff 已保存，黄金样本不代替独立批准。

## 验收结论

SEM-A01–A07 的所选自动验证与独立审核已完成：公共文件/请求路径、严格接口与缺材料阻断、链路独立 Current、共享依据撤销/回退、原生命周期与准入、PS5.1 七类结果及展示回归通过。L2 场景清理已核实。未提交或导出 Git 历史测试记录，dirty 工作区结果仅作为本次实现证据。

## 提交前审计与 main 集成（2026-11-02）

用户追加授权“审计后提pr”。新独立只读代理 pre_pr_audit 核对实现结束时41文件与 final-verified 字节/SHA账本零差异，核实四份正式通过报告，批准实现及提示词/接线变更提交 PR。该账本保留为初轮实现结束快照，不改写其原受测版本。

随后将功能提交 rebase 到 main `02285e1cb9b62564d37ee8a03a0f2f839af4c482`（#434 环境页面精简），得到 `b1fb0b8e6893971c2ce8cd0c25e44e7a608936a1`。仅验收索引追加冲突，保留两边记录；两个 Web 测试自动合并。独立 range-diff 复审批准集成，公共路径生产代码/脚本/提示词维持原审字节，历史 created_at 与新页面行为保持。

在该干净提交补验以下项目，原始报告仍位于本 scope：

| 范围 | 输出 | 结果 |
| --- | --- | --- |
| app 环境材料/历史/配置/单次调用及共享资格、prompts 完整正文 | diagnostics/pre-pr-20261102T113846926Z-9e73d7a4f6594c02a05e2c1ec6b67bbe | passed，零跳过/零失败，212.843秒 |
| web.unit | test-results/L4/pre-pr-20261102T113851523Z-a056d6af63d24ce1a7f4522f4fb68cfc | passed=true、complete=true，managed 为 passed |
| L1 | test-results/L1/pre-pr-20261102T113913137Z-44506fe4af0449ce9441061d18826e2e | passed=true、complete=true，managed 为 passed |
| L2 | test-results/L2/pre-pr-20261102T113929967Z-375d6e5c7b45419598fd57304ecbb7bf | passed=true，smoke.delivery 及 cleanup_complete=true |

go.environment 全集360项的通过仍对应初轮基线加实现快照，不冒称在新 main 上完整重跑；集成变化通过上述定向 owner 和正式 Web/L1/L2 复验。PR 提交前正文/接线完整 diff 和 UTF-8 字节账本另存 diagnostics/pre-pr-review/。后续提交仅补本节和 Plan 的授权范围说明，不改运行代码或测试标准。

## 未验范围

真实 Codex/Orca Runner 正负调用、远端实际工具及人工跨目标体验未执行；受控 owner、脚本直接执行不证明这些范围。安装现场和其他受管项目未改；旧 manifest4 项目须自行正常采用新材料，运行时无迁移/回退。
