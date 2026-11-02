# 执行方案能力投影误判与必填权限修复

2026-11-02。对应[Spec](../../spec/runtime-project-environment-decoupling.spec.md) §3.1、[Plan](../../plans/runtime-project-environment-decoupling.plan.md) P2。

## 现场与修复边界

用户询问本机 Orca＋Codex 无法选择。只读查询安装版公开执行配置：7个已保存开发方案中，本机有3个 Codex直连、Orca＋Pi 和 Orca＋DSH；另有2个远端 Orca＋Codex，没有本机 Orca＋Codex。因此项目页不会列出该方案。同时公开 combinations 的5种组合全部 supported=false，local_helper.available=true。

核对固定 Runner 0.2.0-dev.4 的公开离线 schema：开发方案必填 access_mode=full_access。Web 的能力投影试样遗漏此字段，导致全部合法组合被误判；新建方案及无匹配模板的链路切换也遗漏该字段，随后保存会被原 schema 拒绝。修复只补齐已有开发契约要求的显式权限，不修改真实校验、原方案或执行权限边界，不给缺少权限的真实保存配置补默认值。

只读以安装版公开 schema 验证5个补齐字段的试样均合法，去掉字段均被拒绝；这只证明配置契约，不证明程序、账号、项目环境或实际执行就绪。未新增安装方案、保存配置、重启服务或启动 Run。

## 实施与验证

受测基线 `89130fbc25ed2c2224295d21f3b151baffa91de3` 的 dirty 工作树。能力投影2处与前端3处构造补充必填字段；回归验证合法组合可见、真实缺权限配置仍拒绝，以及只有本机 Pi 模板时新建 Orca＋Codex 会提交显式权限并继承受信 helper。没有修改提示词正文、模型材料或实际 Runner 调用接线。

证据根 `C:/SteerlineDebug/wt-68cd-combination-access-mode/`，原摘要保存命令、源码与终态：

| 实际选择 | 结果 | 相对证据路径 |
| --- | --- | --- |
| 执行配置 owner | 13项通过，4.3秒 | `diagnostics/projection/summary.json` |
| 完整 Web 服务子包 owner：./web/server/... | 59项通过、0失败/0跳过，23.5秒；包含执行配置子包，不能用 go.http 的根包选择代替 | `diagnostics/web-server-all/summary.json` |
| go.http | complete=true/passed=true，104项、0失败/0跳过，23.5秒 | `test-results/L4/http/summary.json` |
| web.unit | complete=true/passed=true，31文件232项、0失败/0跳过，26.1秒 | `test-results/L4/web/summary.json` |
| 最终固定 L1 | complete=true/passed=true，执行44.4秒 | `test-results/L1/final/summary.json` |
| 最终固定 L2 | passed=true/cleanup_complete=true，执行213.4秒 | `test-results/L2/final/summary.json` |
| Web 构建 | npm --prefix web run build 通过 | 原工具输出；不伪造 managed 报告 |

前端定向诊断首次因新建按钮名称写成“新建配置”失败，实际为“新建方案”；修正测试定位后完整 web.unit 通过，未放宽断言。初轮 L1、L2 针对仅后端修复时执行，分别在 `test-results/L1/initial/`、`test-results/L2/initial/`，后者 passed=true/cleanup_complete=true、241.5秒；前端修复后重新选择最终固定集。没有执行真实模型、PR 交付或安装配置修改；HTTP与Web owner、前端请求回归和固定集覆盖本次配置层变化，不重复无影响的项目环境/Runner执行全集。

最终验证后独立只读审核通过：开发模式与公共契约相符，真实缺字段配置仍拒绝，程序/参数/helper边界未放宽；已核对13/59/104/232项及最终L1/L2和清理记录，无确认问题。文档本地链接与diff空白检查通过。
