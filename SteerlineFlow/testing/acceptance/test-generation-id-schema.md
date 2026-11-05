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

## 安装版最终复验

已通过正式 build-deploy（NoStart）构建、停机验证、安装及摘要检查，16.25s succeeded；复用原 Runner SHA256 `4eaa3776f9352b6e634e23cfe3cf146b97d0fe8c763812bbbf58cf00cce0baf8`。安装调用方来源为干净提交 `f4a6a872fb457b2cf3a1810d21b9581dfe5f23c3`，由普通独立进程正常启动，三个项目 ready/degraded=false。项目/Web配置字节不变；execution.json 经正式部署重新序列化，公开的7个profile、模型/effort/环境键、默认方案与原Runner pin逐项一致。未复制数据库、清空历史或修改私有状态。

通过同一正式 maintenance_generate_all 入口，新请求 `e0a1908370374509bd881eeee22e6495` 只重试两个明确失败项；该批 total=2/succeeded=2/failed=0，2026-11-01 23:02:29（Asia/Shanghai）完成。原102/103/104生成ID完全不变，原101/105错误结果仍保留为invalid/stopped，不把原失败改写为通过。

| Task | 最终生成 ID | 实际 Run |
| --- | --- | --- |
| 101 | `63b277799ff44c8dd69d0d50d6450242` | `ba35a6320f5c56b9eab32d7fe1ad0d78` |
| 102 | `214dcd885ee530964c59b6b1a1fe336c` | `3755cfee3b163355b0daa991b055acb4` |
| 103 | `7b305394e1fe62084b6f6774f8496228` | `94499702485106752c9b713fdba80ada` |
| 104 | `6d74ac364493bcd8be61a7311305ea2d` | `138b4f2367ea9e4ee435eeffa5cf4c42` |
| 105 | `d8e76fef55e1260a808ea3a8159c8870` | `5371a402345c727606c083cb5f6227df` |

五项最终材料均ready，各返回该样本CONTENT/SCOPE与demo.progress.roundtrip/retry/cleanup五个精确ID，全部state=generated/stop_status=stopped。正式maintenance_update已快进本地main至原配置目标 `74d7b3a8ace574d17cb436b2a4cf5ed8d82835a5`，local=synced；正式任务仍全open，manual+paused，无业务样本派发。

另按用户选定基础标准实际检测本机默认环境一次：活动 `e3a072d4499b4a04a623b226745e9dad`，Run `d6add5b0c2d3da3412292dea0bf03902`，最终ready/settled=true，原执行completed/stopped/agent_reply_verified=true，result collection_status=closed（2956 bytes），报告completed=true/ready=true并保留原来源和摘要。本机Python/Git+完整L1/L2通过，不外推到其他三个环境或TTY/PR交付。

最终原始证据：`schema-deployment-timings.json`、`application-after-schema-update.json`、`configuration-preservation-verified.json`、`one-click-schema-retry.json[.request.json]`、`maintenance-final.json`、`generation-final-101…105.json`、`snapshot-final.json`、`project-main-fast-forward.json`、`default-environment-progress-01.json`。执行配置文本摘要断言过严导致的本地核验中断发生在重试前；随后通过正式公开profile/环境键和原pin核对语义，才提交唯一重试请求，未绕过产品门禁。只读环境选项计算耗时较长如实保留，不宣称页面性能已验收。

材料PR #748已按用户明确授权合并；调用方修复 [SteerLine PR #422](https://github.com/erranliu/SteerLine/pull/422) 保持待审，不自动合并。本记录后续提交仅补实际验证文档，不重建或改写已验证安装包。
