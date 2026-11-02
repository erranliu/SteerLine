# Plan：共用环境材料实施与验证

2026-11-02。[Spec](../spec/shared-environment-materials.spec.md) · [验收](../testing/acceptance/shared-environment-materials.md)。普通开发直接实施，不进入 roadmap。

## 1. 顺序与交付

1. 冻结 Spec/Plan 草稿并由独立只读子代理审计；修复确认问题后取得明确审计结论，再开始实现。审计记录归专属验收。
2. 修改 environmentadaptation/model 的公共路径 API 和 manifest5，去掉 Manifest.EnvironmentKey；保留 Key/Context 和 report3。修改 app 的读取/请求文档引用/材料展示，去掉文档内 key 匹配。不同目标仍各自计算原稳定上下文与依据。
3. 更新 remote test 前置读取、所有 Go/Web/浏览器夹具、提示词完整正文期望与路径断言。检查 task testing 的环境材料排除随公共入口同步，不扩展文件分类框架。
4. 合并本仓公共要求并交付单一 PS5.1 检测入口，仅声明 go.mod。通过现有结果文件保留所有预置关联字段；有界工具查询、正负消息、自有清理和原子替换。同步当前 Spec/Plan、接口指导、README、paths 与必要导航；历史验收不重写。
5. 新增/调整有意义的 owner 回归覆盖 SEM-A01–A07；执行下表，修复实际失败。整理原始正文 diff、UTF-8 前后字节及业务原因，由独立只读子代理复审实现和提示词接线；确认问题修复后复核闭合。

## 2. 验证映射

遵循[测试指南](../testing.md)、[路径清单](../paths.md)和[验收流程](../testing/acceptance/README.md)。每次测试以 UTC 时间+随机 ID 创建独立输出，scope 为 wt-76f8-shared-environment，产物放 `C:/SteerlineDebug/wt-76f8-shared-environment/`；不覆盖失败、不自动重试，不修改安装现场。

| 验收 | 测试 ID / 入口 | 时点与通过条件 |
| --- | --- | --- |
| A01–A05、A07 | 定向诊断 `go run ./cmd/test-feedback -out <diagnostics/run> -- ./internal/environmentadaptation/... ./internal/app ./internal/testkit/remotetest ./internal/testmanagement/model ./internal/prompts -count=1` | 接线完成后；公共路径、严格接口、跨链路独立 Current、共享依据撤销和原生命周期回归通过 |
| A01–A05 | `go.environment`：`go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.environment -out <test-results/L4/run>` | 定向修复后；正式 owner 全集通过，原 summary/JSONL 保存 |
| A06 | PS5.1 本仓入口正向及隔离工具负向的定向 Go owner 测试（internal/app），从临时工作区执行真实脚本 | 脚本完成后；预置身份/UTF-8/原子结果、缺失/低版本/异常、上下文一致、无链路调用；不把工具不足改报通过 |
| A07 | `web.unit`：`go run ./cmd/test-feedback -level=L4 -suite=web -scenario web.unit -out <test-results/L4/run>` | Web 夹具更新后；正式前端全集通过；浏览器路径夹具更新但不声称浏览器实际执行 |
| 全部回归 | `l1.minimum`：`go run ./cmd/test-feedback -level=L1 -out <test-results/L1/run>`；`smoke.delivery`：`go run ./cmd/test-feedback -level=L2 -out <test-results/L2/run>` | 实现收敛后串行执行，summary/managed 完整通过且清理核实；Go/Web 构建共享目录不并行 |
| A02、A07 | `go test ./internal/prompts -count=1 -v`、`python scripts/check_doc_links.py`、`git diff --check` | 最终审计前，原正文变化与字节账本可复核，链接/格式无误 |

不新增测试清单 ID；复用 go.environment、web.unit、L1/L2。若改变 HTTP 契约或服务逻辑再按指南补 go.http；本次仅公共路径响应值变化，沿原 owner/API 测试验证。无真实模型/远端/安装操作授权依赖，不因未验远端而伪造结果。

## 3. 审核和结束条件

提示词模板预计不变，实际完整调用正文路径与黄金样本随公共入口改变。按[提示词维护](../../docs/prompt-governance.md)保存完整 Git diff、UTF-8 字节前后值、业务理由、测试结果和独立批准；更新黄金样本不是批准。审计发现问题修复后重新核对最终字节。

仅在实现、所选自动验证和最终独立审核完成后报告完成；原生链路和人工体验另标未验。用户未要求提交、推送、PR 或部署，本轮交付工作区改动、Spec/Plan 与证据记录。
