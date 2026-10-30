# 验收：旧 Runner Check 接入退役

2026-10-29，实施、测试及独立复审完成。[Spec](../../spec/legacy-environment-check-retirement.spec.md) · [Plan](../../plans/legacy-environment-check-retirement.plan.md)。普通开发，无 Roadmap Task ID。未部署或操作安装环境。

## 基线与边界

源码HEAD `7617cddc7efac5716c8c38bb63a0157a100acac0` 加开工前未提交readiness修改为受保护基线。基线记录2192文件的字节数与SHA；本次删除23文件，不reset或覆盖既有工作。SQLite由34升35，旧格式直接拒绝，无迁移/兼容读取。

项目文档/脚本、固定提示词、普通Run/result、当前准备引用和派发门禁保留。旧业务、模型、表/索引、HTTP/Web、Runner Check消费及启动/占用/接管/停止/reset耦合已删除。剩余名称仅为新Web文件所在目录、拒绝旧入口的负例、项目报告普通checks及历史记录。历史源码链接固定到原提交，未改写原版本结论。

全部输出根 `C:/SteerlineDebug/wt-5069-legacy-check-retirement/`；下表路径相对此根。测试命令唯一维护于Plan第3节。未运行模型、真实PR或三条原生准备；原生未通过项仍归[readiness验收](project-environment-readiness.md)。

## 自动验收

| ID | 实际范围 | 状态与证据 |
| --- | --- | --- |
| LCR-A01 | 旧调用/资产/入口删除，当前消费者与历史边界 | 通过；固定源码与资产独立复审，无旧生产消费者或兼容空壳 |
| LCR-A02 | 新活动/候选互斥、缺资格/失效门禁、普通Run/result | 通过；最终owner及readiness L3，普通Run/result准备、精确门禁与失效拒绝 |
| LCR-A03 | 原Run/请求/input/实例、未知零重派、停止/接管/重启 | 通过；最终完整owner、45项生命周期定向及runtime/recovery L3，原请求/实例/输入/停止/结算保护保持 |
| LCR-A04 | 旧入口退出、新环境页刷新、准备与任务门禁 | Web生产构建、31文件221前端测试及HTTP/server通过；`diagnostics/web-pwsh-db2f387186cb4e91b7595b8ab9862235/` |
| LCR-A05 | schema35无旧表、拒绝旧库、reset路径/Run保护 | 通过；最终SQLite/driver owner，schema及reset反例实际命中 |
| LCR-A06 | 固定L1/L2、三项L3，非零命中、零测试skip及清理 | 通过；固定L1/L2、Plan三项L3全部实际命中且零测试skip，场景cleanup_complete=true |
| LCR-A07 | 固定新Runner、空闲服务启停、原实例/stop结算及离线证明 | 1测试0skip通过；`diagnostics/published-contract-6cb7475876da4c2694c37a85b56773f9/summary.json`；无Agent/业务输入，namespace已不存在 |
| LCR-A08 | 文档静态、实际快照及资产独立审核，无兼容/空壳 | 通过；两轮固定快照独立审核、实际正文/字节账本、文档链接及diff静态通过 |

## Runner真实契约

只读取得Runner清理会话已生成的最终独立EXE，复制核验SHA到 `packages/runner-contract-cc5c2b07c4ef47f599f900da84522c34/`，未自行修改或构建Runner。保留build元数据、原始version及离线capabilities。

| 固定输入 | 实际值 |
| --- | --- |
| Runner / resource version | `0.2.0-dev.4` / `3` |
| EXE SHA256 | `7b57b06472ae7c36befa71222cdc6e61f7bc2fda410adee6d6aaf32e58fdc2b5` |
| resource digest | `2db7c54cc80821239b0d2b59a00f9378c718086b824d5770ea90a730bb7a1068` |
| source provenance | `af29ee471587d7eef41a5eb9ff7d52aefb815183` + dirty；source digest `1fc19cd53089c9c827018c619165c50885902e42aa8053c80b6cea8aad5ac2df`，提交号不能单独代表该构建 |

A07核对静态公开信封及无三个旧capability键、Catalog绑定原实例、full-binding无旧命令；原stop request/instance结算合格后，核对raw stopped JSON无checks、Runs为空、离线takeover为空，再沿路径保护删除namespace。实际目录 `C:/slr/algari5hgqhwxjpts324u7fzlm` 已只读确认不存在。不据此证明原生环境准备成功或替Runner其余测试签收。

## 独立复审与资产账本

实施前审计修正：新契约前置只约束相关wire实现及真实联验；owner明确排除三个外部前置测试并单列，不以默认skip冒充通过。实现预审修复Web深链接、A07原服务结算、fixture原Build/轮次、raw JSON与binding/takeover覆盖。

独立只读审阅者 `/root/legacy_retirement_review` 已逐项核对90项task_delta、23项删除及37个保护文件，未发现确认缺陷或兼容/空壳结构债。保护范围含 `internal/prompts/**`、`internal/environmentadaptation/**`、两份固定环境材料及原环境生命周期代码，均与开工基线一致。最终代码快照 `diagnostics/snapshot-6f09519a1e4f455f9d591f4022206b3e/snapshot.json`，SHA256 `d1883761ef0442061929b3a8165ef70289dc720e3ccd60549c01295d146874c9`。此前快照 `snapshot-7311f72311914d3e9794df6a86daf3e6` 仅A07测试argv不同；增量复审已通过并核实A07原始1项pass、0skip、complete=true、exit0。

每个快照保留baseline、原dirty/current diff、未跟踪文件、实际assets.diff/assets.json。固定模型正文的本轮差异为0字节，原未提交正文逐字节保留。

| 本轮资产/接线 | 开工前字节 | 本轮后字节 | 原因 |
| --- | ---: | ---: | --- |
| `internal/app/runner_preset_test.go` | 20775 | 21156 | 退役旧Check断言，原生命周期用例补实际材料/Build资格前置，保留生产门禁 |
| `internal/testkit/protocol/runner_service.go` | 9256 | 9857 | 删除旧Check夹具，补已有result-file及development/2能力 |
| `tests/e2e/internal/controlled/runner.go` | 24988 | 24198 | 使用共享workflow，不重复声明v2，保留Run断言 |
| `internal/testkit/checkfixture/fixture.go` | 986 | 0 | 唯一旧业务消费者删除 |
| `internal/testkit/checkfixture/published-v1.json` | 28671 | 0 | 同上，无替代检测资产 |
| `internal/testkit/checkfixture/v1.json` | 23053 | 0 | 同上 |

独立资产审核已读取实际正文diff。fixture从现有prompts读取固定规则，不inline新行为指令；存储资格fixture明确无外部准备执行，不计作准备通过证据，普通Run/result准备由正式readiness L3验证。

## 保留失败与中间结果

| 原记录 | 事实与处理 |
| --- | --- |
| `diagnostics/owners-318cde4e3466451d89fb3e3bf780a246/` | 旧生命周期夹具缺准备资格，整包默认10分钟超时。补材料/资格前置，owner显式30分钟预算，未改变生产超时或删除断言 |
| `diagnostics/lifecycle-fixture-*`、`diagnostics/lifecycle-final-277e5c0daf104e6eae467bb300c5d6e2/`、`diagnostics/owners-final-d74d256ee77d4255a1e354642ab12cfc/` | 各轮发现通用result能力、固定测试安排与L2不一致、测试包别名编译问题；分别修复，原报告保留 |
| `diagnostics/owners-final-e9ba9d4fc79b4aeea7a8f546c49d55ce/` | 包含修复前预建无pin目录夹具及一次Git查询超时；对应最终生命周期定向已通过，原失败不改写 |
| `diagnostics/web-825d77d26be747aaa8d96529f31b1da1/` | 构建/221前端/后端通过，但Windows PowerShell5将正常stderr摘要视为NativeCommandError；PowerShell7完整命令在新目录通过 |
| `test-results/L3/4aec4468683e4abc8cc0817da0bd425e/`、`test-results/L3/9abfdfddc1c6492a81f4d9d71bc7a6f3/` | fixture重复v2被目录拒绝；共享事实收拢，保留严格重复拒绝 |
| `test-results/L2/d4f89a566866465fa98e3bc3be8ed0dc/` | maintenance HTTP读取超时，原记录保留，未直接判为环境能力缺失；最终串行L2在新目录通过，该超时未复现 |
| `diagnostics/published-contract-47652086e51b41cbaa1277cd85d28d1a/` | 首轮测试给静态capabilities附加state-dir，服务创建前被拒绝；修正精确static argv后通过 |

中间通过：L1 `0ab92dd35a3445afb70c6ef7213bc309`、`8641ed255a434a128fb76c8ea3ae5adf`；L2 `e9b4890775324693864b92a4fd2587ee`；runtime L3 `de67205afd1b44debb32db135f5e93b6`；recovery L3 `f776ccb597684bc8b71cfd3c71ba1ffb`；readiness L3见A02。最终批次只追加结果，不覆盖原报告。

## 最终批次

所有下表原始报告已核对passed，owner与A07均complete=true、exit0、实际非零命中、0测试skip；L2/L3场景清理完整。无测试文件的包不计作测试skip。

| 范围 | 原始报告（相对输出根） | 实际结果 |
| --- | --- | --- |
| owner | `diagnostics/owners-verified-c753d7afa5734145ada265091bdbbd08/summary.json` | 586项、0skip |
| L1 | `test-results/L1/d64d011e27f94d2d94f54517c89c75d3/summary.json` | 正式选择通过 |
| L2 | `test-results/L2/4f41a21569f048d9948a63773666eb11/summary.json` | 正式选择通过 |
| runtime | `test-results/L3/bcf90b52d497471799864e745d4e841c/summary.json` | 正式选择通过 |
| recovery | `test-results/L3/0396257360bb46218dad562a42562db9/summary.json` | 正式选择通过 |
| readiness | `test-results/L3/8832b34a4c364487a2aa41cc59817cdf/summary.json` | 正式选择通过 |
| A07 | `diagnostics/published-contract-6cb7475876da4c2694c37a85b56773f9/summary.json` | 1项、0skip |

源码及模型正文在最终批次期间无后续行为修改；唯一A07静态argv修复由该独立真实测试和增量复审覆盖。owner明确排除的另外两项原生外部前置测试仍归原验收，不计为本次通过。原失败和未复现超时均按上节保留，未覆盖或改写。未部署安装环境。

2026-10-29固定快照的最终证据独立复核由 `/root/legacy_retirement_review` 完成：重新计算7份原始summary的SHA均匹配，owner586项、生命周期45项、Web221项及server104项、固定L1全部6组、L2、三项L3和A07均实际通过、零测试skip；L2/L3清理完整。当时源码与最终快照、37个保护文件与开工基线均无差异，未发现确定问题，本次退役范围验收通过。文档收口脚本的错误目标写入被沙箱在打开前拒绝，修复为只允许工作区写入；A07原始summary SHA仍为 `07c12971f15a378829d6d876f1d2f677280e51e8743fa4c0e4ac5ef102714586`，证据未受影响。此结论不涵盖模型/真实PR、三条原生准备、其他原生专项、Runner全仓验收或安装部署。

## 2026-10-30 PR 清理复审

PR完整范围包含此前未提交的readiness及旧Check退役，未仅提交删除片段。复审发现 `environmentPort.ready/readyFor` 只剩测试消费者，已删除这两个生产包装，六处测试直接使用正式 `admission(..., false)`，原成功/失败及候选材料拒绝断言保留；其余旧名称仅为负例、通用checkpoint、历史及承载新页面的目录。readiness验收文档已区分材料子目录和原现场的任务证据根。

实现提交 `a4be7228` 无冲突合入main `a7ec835a`，受测源码提交 `e8997a0dbcad190e73958586210d8da74c5dcda4`。上游纯模型测试归属修复、远端probe显式空测试安排及历史记录保留。`/root/legacy_retirement_review` 与 `/root/readiness_review` 独立批准该源码完整PR及合并增量，未发现其他确定问题；固定提示词实际正文与镜像的范围和字节仍为4257→4263、5488→5500，批准未依赖黄金摘要更新。

下表证据根为 `C:/SteerlineDebug/wt-5069-readiness-check-pr-audit/`，原报告不改写。后续只追加本文与PR说明，不改受测行为。

| 验证 | 实际结果 | 原始报告 |
| --- | --- | --- |
| 合并后L1 | passed | `test-results/L1/c0ad4a930f7c484ea6a8371a379df1ba/summary.json` |
| 合并后L2 | passed、正式场景清理完整 | `test-results/L2/af6eba48f03942559af831b3e0a89be7/summary.json` |
| 环境app定向 | 16项通过，0skip | `diagnostics/environment-increment-78f022459606451a9bc6c219808d2f55/summary.json` |
| 环境model/service、prompts、remotetest、test-feedback | 211项通过，0skip | `diagnostics/model-prompts-tools-85d90a7754e24951bc885fa04e362583/summary.json` |
| 额外e2e标签testenv包 | 11通过、1失败、1跳过，complete=true；不登记整包通过 | `diagnostics/e2e-owner-b76465bf1daf4241a89bfb0979543a3e/summary.json` |

额外失败为 `TestTwoProjectsIsolateTasksGitAndCleanup`：maintenance HTTP查询达到场景期限，根因尚未确定，不据此认定目标环境缺少能力。该报告与此前同类超时均保留，不删除断言、不转换成通过。PR据此以草稿提交，保留待验证状态；此前586项owner、Web、三项L3及真实空闲Runner联验仍属于各自冻结快照，不重标为新提交全量通过。三条原生准备的既有失败结论不变，未部署安装环境。
