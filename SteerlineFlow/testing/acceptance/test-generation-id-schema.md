# 测试安排生成的精确 ID 输出约束（2026-11-01）

用户授权按项目初始化指南补齐 SteerlineTest，并明确授权合并其材料 PR #748 后验证安装版一键生成。材料已采用到 `74d7b3a8ace574d17cb436b2a4cf5ed8d82835a5`；原缺入口/协议不同源的提交前阻断已解除。原公开批次 `16e14040dad84c7181d3c001f77ef468` 实际启动五个生成 Run，102/103/104 成功，101/105 在 `tests` 字符串附加路径/命令说明后被原清单校验正确拒绝。五个原 Run 最终均 stopped，未派发业务样本。

## 修复与审核

原调用方 output_schema 只限制 tests 为唯一字符串数组，允许模型把来源说明写入 ID。现在从本次冻结 Catalogue 注入精确 ID enum；空清单限制 maxItems=0，不制造无效空 enum。原来源、原请求持久化/恢复、结果 Select 校验和已成功结果复用规则不变，不裁剪或修补原错误字符串。

独立只读审核 initialization_guide_review 已批准：Schema 只承载业务 ID 事实，不新增提示词规则；固定 testing-generate.md 为 UTF-8/LF 266→266 字节、正式 testing 资产3256→3256，正文和摘要未改，黄金样本/CI未改。原Schema152字节，SteerlineTest实际16ID Schema428字节，空清单165字节；具体 schema 和新增函数正文随 Git diff 审阅。实际测试从 GenerationService 提交捕获 Schema，使用正式 JSON Schema 实现验证原两种注释错误、未知、重复、额外字段均拒绝，准确 ID 和空清单规则有效。

完整 owner 发现现行主干已遗留的队列夹具错误，在未改 `bf2e100b` 的独立 worktree 精确复现：只改 build.Version 却期待原请求 rejected，与当前仅按 namespace 绑定的生产契约不符。夹具改为不同 StateDir，要求原状态、Unsubmitted、精确 namespace 诊断、原 pin、原请求 ID/Input 逐字保留且零提交；生产只校正旧注释，无行为变更。该调整也已独立批准，不回退生产资格或隐藏原失败。

## 验证与原始证据

证据根 `C:\SteerlineDebug\steerlinetest-initialization-20261101-3701`。此前项目材料、自检及字节审核记录见 [SteerlineTest PR #748](https://github.com/erranliu/SteerlineTest/pull/748)；材料合并不代表本缺陷已解决。

- `schema-regression`：新增真实提交 Schema 回归 passed。
- `schema-L1`：固定 L1 passed，准备0.4s/执行12.9s；随后夹具及最终版本结果另记，不将未执行项算通过。
- `schema-generation-owners-verified`：完整 testmanagement/projectmaintenance owner passed，5.259s；首轮过期夹具失败在 `schema-generation-owners`，未改主干复现在 `schema-owner-baseline-failure`。
- `schema-generation-owners-final` 为修正旧夹具时漏 filepath 导入的编译失败；补齐后才完整运行上述 verified 目录，原失败保留。
- `schema-L2` 首轮在自动材料阶段报 Runner fixture query unavailable，未完成核心闭环；该轮 cleanup_complete=true，不登记通过。后续完整固定 L2、应用一键生成 owner、安装更新及明确失败项的实际补验结果在本记录后续结果和 PR 说明登记。

最终代码验证：`schema-L1-verified` 固定 L1 passed/complete（准备0.321s、执行8.400s）；`schema-L2-final` passed/full-L2、cleanup_complete=true（准备5.796s、执行96.334s）；`schema-generation-owners-verified` 两包55项全部通过、零跳过；`schema-maintenance-owner` 应用一键生成/失败重试/未知请求与固定来源7项全部通过、零跳过，59.230s。最终L1前的 `schema-L1-final` 因只读复现用的嵌套bin/schema-baseline被源码守卫识别而失败；核实该自有worktree干净后正常移除，再完整重跑通过，未调整守卫。Markdown导航与diff检查通过。

本修复仅更新调用方生成约束，不升级 Runner、不清理安装历史或数据库；安装复验必须通过正式停机/验证/替换/启动入口，保留三个已成功生成和两个原失败。新失败重试采用新的请求身份，旧结果保留。真实 TTY/PR 交付仍在本次基础环境验收范围之外。
