# SteerAgent 全局配置验收

2026-11-12。受测基线 `f0d88af0` 及本工作区变更。[产品规则](../../../docs/product-design/execution-configuration.md#调用方设置与环境边界)

## 验收范围

- 在「配置 → SteerAgent」选择本地 Codex 模型和思考强度，验证目录校验、草稿保留、条件保存及重启生效提示。
- 核对新建对话建议、Roadmap 解析、文档生成和多轮规划读取同一个 `steer_agent`。原请求保持冻结，明确失败后的单项重生成使用当前配置和原确认材料。
- 在受控维护现场为 SteerAgent 和 Runner 配置不同模型及思考强度，核对实际提交的调查、审计和收尾决策使用只读配置，实际修改、验证和交付使用所选 Runner 执行配置。

## 验证记录

| 检查 | 结果与证据 |
| --- | --- |
| L1 固定集合 | 通过。报告 `C:/SteerlineDebug/wt-bc56-steer-agent/test-results/20261112-084530-84cf6be0/L1/summary.json` |
| L2 核心交付冒烟 | 通过。报告 `C:/SteerlineDebug/wt-bc56-steer-agent/test-results/20261112-084530-84cf6be0/L2/summary.json` |
| L3 受控维护 | `runtime.refactoring-maintenance` 通过。实际提交的只读请求使用 `fixture-steer-agent / medium`，写入请求使用 `fixture-runner / high`。报告 `C:/SteerlineDebug/wt-bc56-steer-agent/test-results/20261112-084530-84cf6be0/L3-refactoring/summary.json` |
| L4 Roadmap | `go.roadmap` 通过，覆盖解析、单项重生成、真实 Git 发布及规划恢复。报告 `C:/SteerlineDebug/wt-bc56-steer-agent/test-results/20261112-084656-095e7219/L4-roadmap/summary.json` |
| L4 HTTP | `go.http` 通过，覆盖配置保存、目录结果校验和旧键拒绝。报告 `C:/SteerlineDebug/wt-bc56-steer-agent/test-results/20261112-084656-095e7219/L4-http/summary.json` |
| Web 定向测试 | 设置、Roadmap、规划及维护组件共 60 项通过。TypeScript 与 Vite 生产构建通过 |
| 设置页浏览器 | `configuration-navigation.spec.ts` 四项通过，覆盖 SteerAgent 的选择、保存冲突、重启提示和窄屏布局。证据 `C:/SteerlineDebug/wt-bc56-steer-agent/diagnostics/20261112-085212-dd2567c0/browser` |
| 配置冻结与严格解码 | 对话建议准备后重启仍投递原请求，新请求使用新设置。旧键和无效设置被拒绝。报告 `C:/SteerlineDebug/wt-bc56-steer-agent/diagnostics/20261112-085212-dd2567c0/freeze/summary.json` |
| Runner 公开配置接口 | 独立 Runner 构建通过 `compose` 为 `generate/1` 和 `conversation/2` 分别生成 `read_only` 和 `full_access` 配置。两种工作流均返回默认的 `gpt-6.1-sol / high` 与显式选择的 `gpt-6-luna / medium`，后者覆盖了基础预设的模型和思考强度，并保留其本机 Codex 可执行文件 |

验证使用受控执行和 Runner 的无服务配置接口。真实模型运行及安装部署未执行。
