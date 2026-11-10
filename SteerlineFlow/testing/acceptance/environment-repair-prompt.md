# 复制修正环境提示词

2026-11-10。按用户要求直接实现，基线 `c8feff352d4cbd19c93f6f2f89d2fd283e1c115e` 加本次工作树差异，未部署。

项目运行环境页面顶部新增“复制修正环境提示词”。默认 Agent 已在目标机器的项目目录，所有环境共用准备文档；不拼入机器路径、检测诊断或执行规则。复制成功显示反馈，剪贴板失败时展示完整正文供手动复制。提示词由既有配置查询加载，不依赖检测结果；读取失败时按钮禁用，不提供内联兜底。

## 提示词正文与接线审核

新增固定模板 [environment-repair.md](../../../internal/prompts/environment-repair.md)，由同包 `EnvironmentRepair` 组装，业务端仅提供 `model.DocumentPath()`。既有 `/environment/options?configuration=true` 返回 `repair_prompt`，前端直接复制服务端正文。没有新增规则材料，完整准备要求仍由 Agent 从目标项目文档读取。

实际正文由不存在变为以下全文（UTF-8：0 → 127 字节，无尾换行）：

```text
请阅读项目环境准备文档 `SteerlineFlow/environment.md`，根据文档要求完成当前机器的项目环境准备。
```

模板新增全文如下，UTF-8：0 → 117 字节（含 LF）：

```text
请阅读项目环境准备文档 `{{.DocumentPath}}`，根据文档要求完成当前机器的项目环境准备。
```

业务原因是让用户将简短修正请求交给目标机器上的 Agent；属于新增复制用途，没有替换既有检测提示词。黄金样本保存实际 127 字节正文，独立审核同时检查实际模板及 API/前端接线，不以更新样本代替批准。

独立只读子代理 `review_environment_repair` 已审核并批准。初审发现组件测试用 `import.meta.url` 读黄金样本时得到非 file URL，导致套件加载失败、0 项执行；改为 `node:path resolve` 后组件测试 29/29 通过，原审核者增量复核通过。

## 验证

证据根为 `C:/SteerlineDebug/wt-3956-environment-repair/`，下列路径相对此根。正式报告记录受测提交及工作树差异；后续仅修正组件测试读取路径与补本文，既有通过证据按不受影响范围复用。

| 验证 | 结果与证据 |
| --- | --- |
| 提示词专项 | `go test ./internal/prompts -count=1 -v` 通过，实际正文/黄金样本均为 127 字节；包含缺文档路径拒绝测试。原终端输出。 |
| 页面组件 | `npm --prefix web test -- src/features/environment-checks/project-environment.test.tsx`：29/29 通过；包含检测状态未读出时复制、剪贴板失败手动复制及配置失败禁用。原终端输出。 |
| 生产构建 | `npm --prefix web run build`：TypeScript/Vite 通过。原终端输出。 |
| L1 | 通过，`test-results/L1/20261110T080559200Z/summary.json`。 |
| L2 | 通过，`test-results/L2/20261110T080658296Z/summary.json`。 |
| 环境专项 | `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.environment` 通过，`test-results/L4/20261110T080539781Z/summary.json`。包括配置查询在源码/存储不可用时仍返回正式提示词的断言。 |
| Edge 浏览器 | `npm --prefix web run test:e2e -- environment-overview.spec.ts`：1 项通过，`diagnostics/browser-20261110T080647329Z/`。验证复制内容与黄金样本完全相同、成功反馈、320–1440px 共六个视口按钮可见和页面无溢出、零业务写入及零页面异常。 |

已查看 `overview-1440.png` 和 `overview-320.png`，按钮、反馈和环境列表显示正常。浏览器使用生产前端及隔离 HTTP fixture，不代表真实模型执行、安装准备或环境检测通过。

L2 报告确认 `cleanup_complete=true`。文档链接检查通过（742 文件、4627 条本地导航、零错误），`git diff --check` 通过。
