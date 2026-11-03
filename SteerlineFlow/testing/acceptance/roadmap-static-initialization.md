# Roadmap 静态初始化闭环验收

归属 [Spec](../../spec/roadmap-static-initialization.spec.md)、[Plan](../../plans/roadmap-static-initialization.plan.md)。2026-11-02 用户授权。证据根 `C:\SteerlineDebug\wt-762b-roadmap-init`，下文使用相对路径。

| 项目 | 状态 |
| --- | --- |
| RI-01 首提交、初始化边界 | 通过；`diagnostics/preparation-owner-final-20261102T184830250Z/summary.json` 九项含子例，包含真实 Git 的新项目首提交、已有初始化、原版本/目标/严格payload与merge阻断。 |
| RI-02 固定来源、未发布/无效指南/轮询 | 通过；同上。工作树和未发布 commit 不授予 ready；非法UTF8/超限已发布指南拒绝；健康轮询仅启动两个引用查询，业务仍独立读取固定原文。 |
| RI-03 原请求对账、项目隔离 | 通过；`diagnostics/recovery-final-20261102T185521758Z/summary.json` 七项父/子用例，96.823s，零 fail/skip；真实SQLite原pending覆盖缺失、部分、完成、HEAD变化、Git权限故障、查询中实际取消。前四种按只读事实结束；后两种保留原pending，恢复读取后原号才能完成；查询零文件写入/Task创建。跨项目按钮反例与当前项目未知请求隔离由Web十二项定向及全集验证。 |
| RI-04 项目材料PR及实际六节点整链 | 通过；[SteerlineTest #751](https://github.com/erranliu/SteerlineTest/pull/751)，项目L1五项/L2一项通过；真实六任务整链详见下文。 |

原始失败保留：首次 merge 反例尝试用 Git update-ref 写 MERGE_HEAD，被 Git 拒绝；改为本次临时仓库真实伪引用文件，未改产品 guard。首次 pending 夹具缺 RequestedBy，被正式 journal 校验拒绝；补齐夹具，不放宽校验。独立审核要求修复跨项目 pending、原请求恢复、固定指南编码/大小及重复读取，实际结果完成后追加。

`diagnostics/recovery-boundary-20261102T185232087Z` 的缺文件反例曾因 `os.IsNotExist` 不识别 fmt 包装失败；改为 `errors.Is(err, os.ErrNotExist)`，不扩大未知 I/O 的终态分类，最终七项复验通过。普通沙箱无权限读取外部 clone 的 Git blob 不计业务失败；相同只读比对在具备授权的独立上下文通过。

本轮不改变固定短提示、完整规则或黄金样本。静态资产唯一来源仍为 internal/prompts/materials；平台解析规则766字节、文档规则1676字节、默认指南4250字节均保持。SteerlineTest 新增资产各从0变为上述字节数，同源SHA和实际PR在最终记录登记。初始化仅提供文件，无模型规则拼接或 inline 兜底。

## 材料采用及实际正文

SteerlineTest 准备分支材料提交 `16adfb2`，最终 PR head=`b378559a076666492bad51f496f2811855de94b8`。三份目标 Git blob 与 SteerLine 的唯一资产原始字节完全相同；初始化说明和有限导航说明正常发布到 main 的前置，不新增动态 JSON，也不修改原101–105任务/历史evidence。

| 实际材料正文 | 平台字节前→后 | 项目新增字节 | SHA-256 |
| --- | --- | --- | --- |
| roadmap-parse.md / parse-rules.md | 766→766 | 0→766 | c1452c0d591caf5cf6f3db0f4d156c2e1a4451c797e96837c0afb1d6699463e6 |
| roadmap-documents.md / document-rules.md | 1676→1676 | 0→1676 | 4f04f38c14ba54d90d6ebb0000f6a21652a5dd224afd074efd9a10a909ff827e |
| roadmap-spec-guide.md / SpecGuide.md | 4250→4250 | 0→4250 | 140acc1062a7838474a11947b55087c8c5194a18ec0be9cd5c8c313db92f230b |

业务理由：补已有安装项目未发布静态材料的缺口。原短提示正文和其字节数也未改变；只补准备入口、就绪事实和原请求恢复，不替换规则、不内联长材料。独立只读审核 `review_roadmap_newlines` 审实际接线、准备所有者、真实日志及三份源资产，发现修复补验后无剩余可执行问题；不以更新样本代替审核。

项目正式报告留在独立 clone：`.steerrunner/artifacts/l1-ee1aeff694ab4a9dbc6bb4b33b93f435` 和 `l2-f648ac6bb4984aa589eecb7e6f2c5295`，采用源提交16adfb2，分别5项/1项通过、零fail/skip；PR后续提交仅登记材料结果。真实安装项目 main 及活动数据库未作为测试现场，材料PR需合入后正常更新本地再由安装版采用。

## 真实 sample-demo 转正式 DAG

正式定向入口 `go run ./cmd/test-feedback -out <新目录> -- -tags=e2e ./internal/app -run '^TestRoadmapPreparedSteerlineTestLocalCodexPromotion$' -count=1 -timeout=25m`，固定 `STEERRUNNER_EXE`、`STEERLINE_ROADMAP_SOURCE_REPOSITORY`、`STEERLINE_ROADMAP_EVIDENCE`；源是上述PR的精确提交。读源只 fetch Git 对象到本次 TempDir，重置的仅是本次独占临时 checkout，源仓库不写入。使用正式初始化action→提交/推送→固定来源就绪→解析确认→文档生成→原发布恢复/合并→入图恢复路径，非安装版旁路。

`diagnostics/actual-six-node-20261102T184835604Z/summary.json` 通过，437.136s，零skip；真实 Runner/Codex 采用实际 `sample-demo.md`，严格核对六个内部代号和六条直接依赖，生成十八份 Spec/Plan/验收正文，实际隔离 Git/provider 合并及真实 SQLite 入图成功。mapping：demo-tools→0、demo-seed→1、demo-left→2、demo-right→3、demo-join→4、demo-independent→5；保留分叉/汇合及独立分支，原入图/PR回执重放不重复。它们是隔离权威编号，不替换安装库已有Task，也未执行201–205业务实现。

源码 code_commit=`b378559a076666492bad51f496f2811855de94b8`，文档 merge_commit=`b131b49b4a0d0ca89fdf42ac5afaa6d9d3885170`。解析 Run=`4f340aa4a49c366924b0d1af9b5bf5e6`、文档 Run=`a9dcaece80d615a877a20ac1ea8c6cd3`，均 completed/stopped。`evidence/runner-settlement.json` 公开正常停止核验 resources_settled=true。独立 Runner EXE SHA256=`41de9144c4c999298628e24511f24767f5f3cb5d2959b23edfd80fc55b35be08`，resource digest=`a968df6ee06ae6a16650ed9f0eee3ab878e9406a4541ab44fa6548674161c707`。原请求、完整JSON、文档正文及来源manifest保存在同轮evidence。原生场景在恢复错误分类最后一行修正之前编译，该修正只有异常查询路径，未冒称原生重复执行。

## 系统回归

固定 L1 `test-results/L1-final-20261102T185829182Z/summary.json`、固定 L2 `test-results/L2-20261102T185536183Z/summary.json`、前端全集 `test-results/web-20261102T185234758Z/summary.json`、HTTP登记集 `test-results/http-20261102T185420258Z/summary.json` 全部通过，生产前端构建通过。文档656文件/4078本地链接及diff检查通过。真实外部文档PR的合并、安装部署、业务执行不在本轮隔离验证通过范围内；当前项目材料PR本身是真实GitHub写入，保留待审阅状态。

## Rebase 后最终回归

本分支已 rebase 到 main `02285e1cb9b62564d37ee8a03a0f2f839af4c482`（环境页面精简），仅验收索引冲突，保留双方条目。精确受测提交 `fe57c6d22fc41dbb655cdb799244ab7b90fc5d24`，干净工作树；`test-results/L1-rebase-20261102-final/summary.json`、`L2-rebase-20261102-final/summary.json`、`web-rebase-20261102-final/summary.json`、`http-rebase-20261102-final/summary.json` 均通过，零意外 skip，生产前端构建通过。文档657文件/4079本地链接零错误，diff检查通过。本段为验证后文档登记，不变更受测业务；main 无 Roadmap 行为变化，未重复调用模型。

## PR #435 冲突解决与回归（2026-11-03）

Rebase 到 main `e5cd4f2a58a713eb1c769604ffabe733b336aa1f`；唯一冲突为验收索引同处新增，保留公共环境材料与 Roadmap 两条记录。range-diff 及逐文件比对确认原业务代码、提示正文、资产和专项测试正文无变化。受测提交 `2342eacb94c9bc879677787a59fb6aa9e59bcf05`，干净工作树。

本任务证据根下 `test-results/L1-pr435-rebase-20261103/summary.json`、`L2-pr435-rebase-20261103/summary.json`、`web-pr435-rebase-20261103/summary.json` 均通过；`diagnostics/owner-pr435-rebase-20261103/summary.json` 11项含子例通过，零失败/跳过，112.252s，覆盖首次提交、固定来源、原请求恢复和指南校验。文档659文件/4094本地链接零错误，diff检查通过；未重复调用真实模型或操作安装现场。本段仅登记本轮结果。
