# Plan：App 领域专项与实际执行参数报告

日期：2026-11-03。状态：已实施，自动验收与独立审核通过。依据 [Spec](../spec/app-domain-test-suites.spec.md)，结果登记到[专属验收](../testing/acceptance/app-domain-test-suites.md)。普通开发，不创建 roadmap 任务；用户已授权实施并在独立审核通过后提PR，重型用例仅梳理。

## 实施顺序

### 1. 固定受测基线与完成断言矩阵

基线为 `4920f452d2358b62c3230f7b5f4a04516a4aa862`。实际实施前确认工作区与上一轮变更依赖，固定集成 commit、dirty 状态、Go/Git/Node、平台及缓存条件。只处理真实冲突和直接领域归属漂移，不为无关 main 变动反复切换基线。若直接范围已变，先更新明确映射并审阅，禁止静默漏选或拿旧版本报告通过。

在仓库外新独占 scope 保存普通 Go `(package, root)` 清单、平台文件/标签与23项提示词相关资产的原字节/SHA；清单枚举只计准备，不计测试通过。不为获取基线再次跑旧完整 App 或 Full。冻结四个重型根及其辅助调用的源码身份、实际子例/循环分支，完成 ADT-A5 矩阵：断言、前置、动作、错误条件、验证层级、外部资源和清理归属逐项对应。首轮梳理必须先于实现；本轮矩阵结论不得触发用例改写。

### 2. 接入两个明确的 L4 领域选择

复用 `localSuite/localGroup`。建议独立小文件维护同包固定源/根映射和漂移检查，避免扩张成通用自动测试发现框架；既有 environment 定义及守卫保留。go.deployment 为 host全量、Windows localservice全量、App9/8根及runnerclient升级排空原根；go.roadmap 为 roadmap owner全量、App11根、host1根，全部串行。每个实际组保持原local命令参数；不扩张全局默认选集。

检查源文件、明确根、辅助文件新根、平台与live排除项；对缺文件、根增删/改名、新前缀文件、标签/平台变动及定义多/漏根，给出需维护清单的错误。接入原 L1 清单检查与两个正式入口。增加真实语法源码 fixture 负例及平台选择反例，不通过编译当前平台来掩盖另一平台源码漂移。已有跨包同名、原始事件唯一性及失败传播回归继续执行。

### 3. 修正实际参数报告，保持执行命令原样

在原报告结构增加同义的每组参数证据，解析器显式区分 `go test`、test2json包装、冻结二进制与Web。优先复用 runGroup 的最终 argv；E2E复用最终二进制参数、真实build记录与原包worker安排；诊断直接用原goArgs。Full的三条分区流水线及其他包流水线分别记录，不能把 `Workers`、Go `-p` 与测试 `-parallel` 混为一项。

GOMAXPROCS 记录来源与主体，顶层只对相同适用维度汇总；混合写mixed，未观测/未执行/不适用明确。补齐 `-flag=value` / `-flag value`、test2json `-p`包名、重复参数的实际有效值、非Go/不完整命令、组间不同参数及失败前报告的反例；不重写命令或改变调用次序来取得统一值。

验证原始命令→每组参数→顶层摘要可追溯；summary变更仍由原managed证据SHA覆盖。保留原失败和进度规则，缺新参数不能由旧default字段兜底。现有报告格式仍按当前版本读取，不新增历史迁移/双读。

### 4. 同步正式文档与清单

实现可执行后再将两个新ID写入 `SteerlineFlow/testing/test-list.json`、`testing/l4.md`、项目 `testing.md`；每项自足说明环境、完整命令、固定范围、live排除、非零/零skip、资源隔离和依赖。更新 `paths.md` / `testing/reports.md` 的实际组目录及参数字段说明。普通Go集不进入E2E manifest；L1/L2固定根与L3场景不变。

本次涉及测试接线和提示词对应守卫/清单，按AGENTS及提示词维护要求，给出实际正文diff（预期为空）、固定Markdown/黄金/组装/writer/embedding/CI的前后字节与SHA；黄金按原字节含CRLF核对。既有L1提示词与边界完整执行，禁止改黄金代替审核。

### 5. 自查、必要验证与独立审核

实现修正结束后按下表执行。全部输出遵循[路径清单](../paths.md)，每次调用新叶目录，报告失败原样保留；不自动重试，不停止其他测试。Full、压力与E2E本轮串行，等待已存在测试自然结束；记录实际外部负载，若只验功能不声称性能对照。首次失败先定位，超出本Spec的生产或夹具修改先形成单独范围，不放宽原断言/期限。

最终审阅代码/定义/原始证据与断言矩阵。提示词相关接线及交付按项目要求独立审核；本Plan不自动启动审核代理，也不代替后续用户实施授权。独立审核通过后才能按后续授权提交/提PR；不自动合并或部署。

## 自动验证计划

所有命令从实际repo根执行。`<out-X>` 为仓库外同一scope下全新、互不重复的绝对叶目录；长集使用当前Windows环境、Go/Git，L1/Web另需Node24+及已安装依赖，不自动联网补依赖。Linux/macOS的选择只做源码fixture/参数逻辑检查，不冒称相应平台原生通过。

| 验收 | 时点 / 测试ID与命令 | 通过条件与证据 |
| --- | --- | --- |
| A1/A3 | 步骤2后：`go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.deployment -out <out-deployment>` | host与localservice活动owner全量非零，App9/8根及子项完整，0fail/skip；summary、原始JSONL、managed-go.deployment、真实隔离目录/清理事实。 |
| A2/A3 | 步骤2后：`go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.roadmap -out <out-roadmap>` | roadmap全量、App11根/host1根精确匹配；LF/CRLF及目录拒绝真实执行，live根零执行，0fail/skip，managed-go.roadmap与原始终态对应。 |
| A3/A4 | 步骤2/3后：`go run ./cmd/test-feedback -out <out-feedback-owner> -- ./cmd/test-feedback -count=1` | 完整执行器owner，包括新选择/平台/标签/原始事件负例及参数解析/mixed/默认/失败传播；经finish报告、managed及进度核验，原测试保留。该调用自身证明诊断参数报告。 |
| A4/A6 | 最终固定L1：`go run ./cmd/test-feedback -level=L1 -out <out-L1>` | 原最小Go/前端/提示词/边界根全部保留，清单阻断漂移；实际每组命令与参数一致，managed正确，0skip。记录完整入口耗时，不要求新增三次性能采样。 |
| A4/A6 | 最终固定L2：`powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/test-l2.ps1 -OutDir <out-L2>` | 核心交付原断言通过、cleanup_complete，冻结二进制/build标签/参数证据真实，0skip；不改smoke步骤或5分钟建议。 |
| A4/A6 | E2E代表路径：`go run ./cmd/test-feedback -level=L3 -suite=runtime -env=controlled -scenario runtime.dual-home-operation -out <out-L3>` | 原两home隔离/请求/零候选及清理通过，核对实际worker、标签与-test.parallel报告。多包worker和不同组参数由执行器受控fixture核验，不为报告接线新增长集并行组合或扩大并发。 |
| A4/A6 | Web：`go run ./cmd/test-feedback -level=L4 -suite=web -scenario web.unit -out <out-web>` | 全部Vitest文件/断言非零、完整、0skip/todo，Go参数标不适用，真实命令保留；不新增浏览器/构建任务。 |
| A4/A6 | 最终Full：`go run ./cmd/test-feedback -level=L4 -suite=go -out <out-Full>` | 全部普通根与实施前清单逐项比对，当前规划620根是参考、实际以冻结清单为准；原根0缺失、完整通过且0实际skip，新增执行器根纳入，编译项独列；真实各组参数、summary和managed SHA一致。 |
| A5 | 步骤1初稿、交付前复核断言矩阵与源码身份 | 四根全部子例/动态分支、关键辅助断言和清理均有对应；原业务测试/辅助源码相对固定基线零diff。仅梳理不执行移层或去重；不以摘要代替完整分支清单。 |
| A7 | 最终：`python scripts/check_doc_links.py`、`git diff --check`，核对清单/正式命令、资产正文/字节及独立审阅 | 文档/JSON/定义相符，无额外E2E条目；治理字节/实际正文和新报告真实性有审计记录，问题修复后复核通过。 |

两专项、L1/L2/代表L3/Web/Full在最终源码稳定后各执行一次即可；前期owner负例不是重跑长集的理由。实现或审核实际修改后按影响补验，不为追求快样本重复旧完整App或性能配对。已有runneracceptance缺输入仍阻断，不能借本轮调整变为skip通过。

## 文档规划轮的交付检查（历史）

此前规划轮只创建Spec、Plan、专属验收规划/矩阵及两处导航。检查本地链接、UTF-8、选择表与源码根名/标签/平台一致、`git diff --check`，正式JSON/Go/CI/资产无改动。规划轮没有运行功能或性能测试；实施后的实际状态另存专属验收，不用该历史段替代当前结果。


### 实施基线集成

实际基线为`cca5114297564df176b27145e66fda71d2582682`，上一轮PR #438已合并。本轮继承主干Roadmap静态初始化四根：`TestRoadmapStaticInitializationAndPublication`、`TestRoadmapStaticInitializationRejectsGitOperation`、`TestRoadmapInitializationPendingReceiptReconcilesWithoutWriting`、`TestRoadmapPublishedGuideMustBeValidUTF8AndBounded`，位于`internal/app/roadmap_preparation_test.go`，全部纳入明确映射；App原7根全保留，当前11根。重型源码按此集成基线冻结，主干已有更新不记作本轮修改；本轮仍不改业务/测试辅助正文。


主干直接部署门禁补充：`internal/runnerclient/upgrade_test.go`的原升级排空根精确纳入go.deployment，原候选证明、两业务读取器与故障子例保留。Roadmap live文件的两个e2e根均显式排除且核对实际标签，非当前平台源码也检查。


## 实施终态

本轮两个L4领域专项、实际参数报告及仅文档的重型断言/层级梳理已完成。最终Full为72包632根1652终态、0fail/skip，原626根零缺失且恰新增6个执行器根；L1/L2/代表L3/Web和完整执行器owner均通过。976个生产/业务测试/资产文件及四个重型根/辅助保持原字节，1760运行文件冻结一致；原断言、期限和调度未放宽。实际命令、旧失败、参数/managed证据和独立终审结论统一见[验收记录](../testing/acceptance/app-domain-test-suites.md)，不新增本轮性能提速结论。
