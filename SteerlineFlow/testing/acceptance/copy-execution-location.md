# 执行现场信息复制验收

普通开发，2026-10-17。范围：在当前候选详情顶部增加两个复制按钮，以最少路径和完整 ID 定位执行现场。

| 项 | 要求 | 自动结果 |
| --- | --- | --- |
| CEL-A01 | SteerLine 复制 home、仓库、Task ID、候选 Run ID；路径/ID 不截断 | 通过；客户端绑定目录与前端精确文本断言 |
| CEL-A02 | SteerRunner 仅配置启用后显示，复制 EXE、候选保存的 state-dir、Runner Run ID、Attempt ID | 通过；配置有/无 Runner、不同候选目录、完整投影与 JSON/HTTP 传递断言 |
| CEL-A03 | 未生成执行 ID 时明确标记；加载前禁用；成功反馈、剪贴板不可用/拒绝后可重试 | 通过；Vitest 覆盖，已有缓存离线时仍可复制 |
| CEL-A04 | Web 类型和生产资源可构建，原执行页用例保持通过 | 通过；构建与执行页定向回归 |

验证环境：Windows/amd64、Go 1.26.4、Node、jsdom、隔离临时仓库和 SQLite。没有启动调试包或访问安装现场；没有模型调用或测试项目业务 PR。此次只增加只读展示字段和剪贴板操作，选择 L1 定向验证，不重跑 L2/L3/L4 或 Full。

实际命令与结果：

```powershell
npm --prefix web test -- src/features/runs/copy-execution-info.test.tsx src/features/runs/runs.test.tsx
# 2 文件，9 项通过，0 失败。
npm --prefix web run build
# TypeScript 与 Vite 生产构建通过。
go run ./cmd/test-feedback -out C:/Users/lmas1/AppData/Local/Temp/steerline-copy-location-go-final-20261017 -- ./internal/app ./internal/product/httpapi ./internal/product/api -run '^(TestExecutionLocation.*|TestFileQuerySourceReadsCombinedProjectView|TestAPIQueryParity|TestTM05ProductDTO)$' -count=1
# passed，19.852 秒。包含公开 DTO、配置副本、候选现场投影及 HTTP 查询传递。
git diff --check
# 通过。
```

Go 原始证据目录为上述 `-out`：`summary.json`、`go-test.jsonl`、`go-test.stderr.txt`。前端结果见本次终端输出。

CEL-M01：实际浏览器剪贴板与窄屏布局体验尚未人工验收，允许后置，不阻塞本次 PR。复制内容为多行纯文本；未生成的 Run/Attempt 标记“尚未生成”，state-dir 缺失标记“尚未记录”。
