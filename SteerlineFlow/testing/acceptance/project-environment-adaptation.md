# 验收：项目远端环境适配

2026-10-26。归属[Spec](../../spec/project-environment-adaptation.spec.md) / [Plan](../../plans/project-environment-adaptation.plan.md)。SteerLine调用方已实施，受控与真实分别记录；不纳入roadmap，不修改Runner或安装现场。

## 本轮：result仅结束后封存一次

按用户提供的Runner `docs/spec/runtime-result-file.spec.md` 2026-10-26最终快照稿修订Spec/Plan并实施。只读来源为 `C:/Users/lmas1/.codex/worktrees/2c8e/SteerRunner`，HEAD为`5df57b69e98382abcd63676b04a73079fd939fa6`加该工作树未提交修订；不是首轮调查的旧Runner目录。复制其公开schema作为本仓受控fixture，`internal/runnerclient/testdata/result-file.schema.json` SHA256为`d226fae7a0d8c95e14c3bbed8afb7708cf9f15f0bc833ea7e2a9059fbe45f2ed`。没有修改Runner或声称该工作树EXE已完成真实联验。

实现仅调整现有result校验、适配观察/停止顺序、提示词及页面：pr_ready/完成/明确失败、取消或超时触发原stop；核实停止后查询原持久结果，pending可继续查，closed后只消费已保存封存事实。version=0/1和disabled/pending/closed独立于progress；closed/0保存并展示diagnostic，不能通过，不重采或resume刷新。只在原Run和PR资源安全条件满足后按剩余三轮预算新建Run。完整L2+完整PR、三轮及二态准入保持不变；未新增表、状态机或通用框架。

下列路径相对仓库外根 `C:/SteerlineDebug/wt-4b14-environment-adaptation/`，本轮源码清单另存`diagnostics/final-only-source.json`。前序持续观察result的测试语义已被替换，下方历史表不计本轮通过。

| 本轮范围 | 报告 | 事实 |
| --- | --- | --- |
| 功能定向 | `diagnostics/final-only-owner-01/summary.json` | 54项通过、0跳过；真实Git/SQLite/API和scripted peer，覆盖停止前零结果读取、pr_ready停止、waiting_input不提前停止、唯一最终报告、封存失败跨重启不刷新、pending/查询故障、三轮重试、二态准入及原请求/生命周期 |
| Runner客户端/适配模块完整owner | `diagnostics/final-only-contract-01/summary.json` | 133项通过、2项跳过、0失败；含原progress回归、新公开result schema/状态/版本/字节/真实空文件校验。跳过TestEnvironmentCheckPublishedContract及TestRunnerFullBindingPublicCLI，因未提供显式真实Runner EXE，不能算真实通过 |
| 固定L1 | `test-results/L1/final-only-01/summary.json` | 通过，准备0.9秒、执行24.6秒 |
| 完整受控L2 | `test-results/L2/final-only-01/summary.json` | 通过、cleanup_complete=true；准备33.8秒、执行160.2秒，实际选择smoke.delivery，受控worker/provider，不含真实PR |
| Web | `diagnostics/final-only-web-02/` | 构建通过、31文件/209测试通过、HTTP/Web服务器回归通过；服务器原报告见该目录server-tests.log，不含真实浏览器 |

Web服务器原始报告为 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261026T044506.906352000Z/summary.json`，103项通过、0跳过。`diagnostics/final-only-web-01`保留新增用例未展开details便断言可见的测试失败；补上展开操作后第二轮通过，产品结果区保持原折叠交互。最终582份Markdown/3650条本地链接检查0错误，变更Go的gofmt、两个页面文件Prettier及git diff --check通过；Web通过后只调整JSX换行格式，没有行为修改。

本轮未运行真实远端Agent、项目验收脚本或真实PR，也未部署；所选Runner Spec自身标记真实远端验收待后续。受控peer及本仓固定L2均不替代项目环境准入所要求的真实完整PR。前序基线actor回归失败仍保留原证据，本轮未重复运行无关app全集。

## 前序来源与实现边界（历史）

源码基线为 `57623186b63c16be2a5748665e63d52ae615adbf` 加当前工作树改动，尚未提交。所有测试使用仓库外独立输出根 `C:/SteerlineDebug/wt-4b14-environment-adaptation/`，每次新建叶目录，原失败保留。最终源码清单见该根的 `diagnostics/final-source.json`。

首轮用户指定Runner已实现双通道作为接入前提；当时调查的旧Runner检出为 `5df57b69e98382abcd63676b04a73079fd939fa6`且未提供result-file。前序受控使用公开progress格式加约定的独立result契约及scripted Runner/PR peer，不能称为已验证的双通道构建。该历史调查不代替本轮所选2c8e工作树及最终快照契约。

五项审计按最小路径落地：预先存在的固定验证base、逐类型生命周期接线、单脚本prepare/publish且唯一PR入口、home公开目标键的窄准入占用、独立环境生成用例。Plan专用GenerationService未增加环境kind，没有registry、主机发现或通用文件/工作流平台。

项目SQLite版本为32，按未发布规则拒绝旧schema，不迁移/回填旧checks。生成内容经正常Git审阅维护后才能用于适配，生成成功不授予ready。

## 前序受控证据（不计本轮final-only通过）

下列路径均相对上述独立输出根；实际命令、源码状态、选择和原始日志保存在各报告中。

| 范围 | 报告 | 结论 |
| --- | --- | --- |
| 功能及相关生命周期owner | `diagnostics/feature-final-01/summary.json` | 44项通过，0跳过；含跨项目/异键、适配并发、原请求、接管及原stop、预算/期限、完整报告与最终快照 |
| 最终URL/报告与双通道 | `diagnostics/report-final-07/summary.json` | 31项通过，0跳过；完整PR、HTTP/HTTPS、坏文件/陈旧Run/读取故障不挂起原停止及期限，未知零重发 |
| 材料/API调用方 | `diagnostics/caller-final-02/summary.json` | 8项通过，0跳过；仓库/材料摘要、普通提交保留、依赖失效、准入及API受理/回执 |
| 固定候选版本准入 | `diagnostics/admission-final-03/summary.json` | 2项通过；候选冻结材料与当前材料交错时阻断，普通源码提交仍保留准入 |
| 生成材料导出协议 | `diagnostics/export-final-04/summary.json` | 1项通过；使用机器段声明路径，不从正文猜解释器 |
| 固定L1 | `test-results/L1/final-07/summary.json` | 通过；准备0.8秒、执行8.0秒 |
| 完整受控L2 | `test-results/L2/final-07/summary.json` | 通过，cleanup_complete=true；准备18.5秒、执行143.6秒 |
| Web构建/前端/服务器 | `diagnostics/web-final-05/` | 构建通过，31文件/208前端测试通过，HTTP/Web服务器回归通过；未运行真实浏览器 |

功能命令使用 `go run ./cmd/test-feedback -out <新目录> -- <所属包> -run 'TestProjectEnvironment…' -count=1`；生命周期合并选择含TestTMGenerationTakeover、TestGQQueuedGeneration、原环境检查及项目关闭/入口排空。API组合使用真实Git/SQLite、项目actor命令入口、journal和脚本化公开Runner/PR peer，证明受理→失败→独立新轮→完整报告→原stop/最终快照→ready。ready预留探针不冒充真实远端候选或原生脚本执行。

## 前序按Spec核对（历史语义）

| ID | 已验事实 | 未验边界 |
| --- | --- | --- |
| PEA-A01 | Git冻结生成输入、稳定提示词、机器段/双文本/gaps、原生结果来源资格、生成不授予准入 | 真实模型生成脚本及各项目实际可执行性 |
| PEA-A02 | 未就绪统一阻断、ready到预留边界、同目标并发/重建、跨项目占用与异键隔离；基础检查入口共用目标门禁 | 真实远端任务与适配竞争；跨home不在保证范围 |
| PEA-A03 | 两通道独立版本，进度全绿不放行，完整L2/本轮/脚本关联、PR必需项、停止后最终快照 | 正式Runner回传来源、真实完整PR及原生收尾 |
| PEA-A04 | 首次加两次、独立Run、重启不重置、不可变截止、超时停止、预算耗尽不放行 | 真机跨服务重启闭环 |
| PEA-A05 | 未知submit/stop零重发、明确拒绝stop恢复、暂不可读取缓存不通过、最终新版本推翻旧通过、取消零后继、takeover及startup stop；坏文件/陈旧Run/读取故障不挂起已核实绑定的停止义务 | 真机未知合并/文件收尾/离线清理组合 |
| PEA-A06 | 有限L2/依赖/仓库/公开配置摘要，普通提交不机械失效；核对候选冻结版本，进度/结果/Agent说明分别展示，原ID只读核实 | 不自动发现环境名背后的机器替换；真实浏览器体验 |
| PEA-A07受控 | 调用方API端口/固定L1/L2/Web受控证据；固定base、唯一publish已接线 | peer不证明原生采集、脚本或真实PR；正式HTTP+浏览器完整远端链待联验 |
| PEA-A07真实 / PEA-M01 | 未执行 | 缺具备result-file的固定Runner构建及实际远端/项目材料/PR窗口，未做真实浏览器签收 |

完整L2与真实PR仍是受管项目环境标准，未改为全部L3。必须有同一轮的完整脚本报告通过；字节摘要、Agent回复和命令applied均不能代替硬验收。最终报告/PR或收尾未知保持not_ready，不授权另起未知写操作。

## 保留的失败与边界

`diagnostics/environment-owner-01` 至 `environment-owner-05` 保留开发失败，分别定位测试peer共享指针、编译/公开provider参数及误用只读store的夹具问题；后续补验通过。`environment-owner-06` 为19项通过，`environment-integration-01` 为24项通过。Web初轮缺依赖，按lockfile安装；第二轮修正测试构造参数；导出用例补上详情读取后，最终208项通过。中间失败报告均保留，没有覆盖或冒充通过。

`diagnostics/owner-regression-01` 扩大回归出现既有 `TestRunnerActorBindsReceiptToOriginalHostFile/changed` 失败。在未修改HEAD的git archive隔离副本中，以 `diagnostics/baseline-host-binding-01/report/summary.json` 复现相同断言，source.json保存归档来源/SHA256。本次没有修改该actor或旧测试。确认基线失败后停止继续执行与功能无关的app全集；宽报告为 **incomplete**，不能声称owner全集通过，所需定向owner另按上表记账。

最终文档检查：582份Markdown、3650条本地导航链接，0错误；git diff --check及变更Go文件gofmt检查通过。这些检查不代替产品或真机通过。
