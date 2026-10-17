# 当前架构与解耦审视记录

日期：2026-10-17。归属：本次用户直接授权的架构文档更新与解耦评估，无 roadmap Task ID。

源码基线：`2fb07ba1e74dfe6b16326d7ed462da9c57a5dec0`。本次只修改文档，未实施建议中的代码重构。现状、三张图和建议归[代码架构](../architecture/current-code-architecture.md)，业务权威归[整体架构](../architecture.md)。

## 核对范围

- 用 `go list` 核对 `internal/...`、`cmd/...`、`web/...` 的当前平台生产直接依赖，并展开 Web 命令的传递依赖。
- 阅读项目运行时资源取得/排空/关闭、Task 候选启动与交付事务、Runner bridge/CLI、环境检查、HTTP 端口与命令仲裁、存储读取与页面投影、Web hook/context。
- 区分已形成的运行边界、残留的编译/类型依赖、同包分文件整理；没有把守卫通过解释成“完全解耦”。
- 校正架构文档中的 schema 21 为当前实现 22；将执行流程所有者明确为 Runner，区分公开 Runner workflow version 与迁出前版本轴。

## 自动验证

| ID | 范围 / 环境 / 入口 | 通过条件 | 本次结果与证据 |
| --- | --- | --- | --- |
| ADR-A01 | Windows/amd64，已有定向 L1 架构测试；见下方命令 | 四项测试、三个包均通过；仓库边界检查包含全标签源码扫描和 Windows/Linux 生产依赖图 | 通过，4/4 测试、3/3 包，13.424 秒；`C:/Users/lmas1/AppData/Local/Temp/steerline-architecture-audit-20261017-073459/summary.json`，同目录 `go-test.jsonl` / `go-test.stderr.txt`。测试在文档编辑前执行，之后未改实现或测试。 |
| ADR-A02 | 文档本地链接与差异检查；`python scripts/check_doc_links.py`、`git diff --check` | 本次变更未引入断链与空白错误 | 通过；462 份 Markdown、2821 个本地链接、0 错误；差异无空白错误。Git 的 LF/CRLF 转换提示不属于检查失败。 |

ADR-A01 的复现入口（`-out` 使用独立新目录）：

```powershell
$auditOutput = Join-Path $env:TEMP ('steerline-architecture-audit-' + [guid]::NewGuid().ToString('N'))
go run ./cmd/test-feedback -out $auditOutput -- ./internal/testkit/boundary ./internal/app ./internal/product/httpapi '-run=^(TestThreeLayerRepositoryBoundary|TestHTTPConsumerBoundary|TestProjectLifecycleOwnershipBoundary|TestHTTPCommandBoundaryArchitecture)$' -count=1
```

以上属于定向集合，不表示完整 L1 或 Full 通过。本次文档更新未选择 L2/L3/L4、Web 运行测试或真机调试，原因是没有运行行为变更；未启动服务或模型，也未使用安装/保留运行环境。Mermaid 图按源码关系核对，未执行独立渲染验收。

依赖采样另存同一证据目录的 `production-imports.txt` 与 `web-dependencies.txt`；本地链接与差异检查输出分别登记到该目录的 `doc-links.txt` 与 `diff-check.txt`。

## 审视基线当时的守卫边界

以下记录本次审视时的源码基线；随后用户授权的第一轮实现已修复 Web 编译依赖、候选错误映射和对应守卫，见[第一轮验收](architecture-decoupling-round1.md)。原审视证据保留。

- `TestThreeLayerRepositoryBoundary` 使用已有仓库边界检查，不意味着已迁出的三层 owner 仍存在。`taskrunner` 已被统计，但旧 `internal/execution` 完整禁止依赖规则未自动等同应用到它；`taskrunner` 当前仍可识别 `sql.ErrNoRows`。
- `TestHTTPConsumerBoundary` 的根列表覆盖 HTTP 客户端、契约和连接包，未包含 `web/server` / `cmd/steerline-web`。本次 `go list -deps ./cmd/steerline-web` 确认 Web 因 `AllowedRead` 依赖后端路由，进而到达 query、storage/ports、execution/model；未到达 SQLite 实现。
- SQLite 的守卫明确允许调用 product/query 的六项纯函数/常量，这些依赖是当前架构事实；守卫通过没有消除投影与存储的职责耦合。
- HTTP command 守卫明确检查同包结构，不声称 command 文件之间存在编译隔离。

本次没有新增功能验收或人工验收门槛；后续若授权实施解耦，再按实际范围安排最小验证，保持原子交付、原请求对账、同快照读取和资源所有权。
