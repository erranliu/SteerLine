# Git 测试历史 Plan

日期：2026-10-23。依据用户确认的 [Spec](../spec/test-records.spec.md)，普通开发，不调度 roadmap。实际结果仅记[本次验收](../testing/acceptance/git-test-records.md)。已读取编写规范、测试指南、验收流程及独立调试流程。

## 实施

1. internal/testmanagement 保留候选所需 Report 解析，替换旧 RecordService/RecordStore 为简洁文件、导出和只读查询；删除旧显式复验与登记对象。
2. cmd/test-feedback 增加 record 子命令，校验同次报告/受测源码/仓库，按 SHA 原子写文件；去掉 record-project/rerun-of。记录目录不影响源码 clean。调试模板的小测试脚本共同切换报告字段。
3. internal/app 读取核实主分支的 Git 目录，按 commit 缓存；当前清单合并仅有历史的 ID。删除 SQLite 表、投影字段及生产者；schema 30，无迁移。CLI/应用客户端/HTTP/公开转发删除历史写入及 summary 路由。
4. Web 保留测试集导航、组合筛选、分页、详情和当前定义；简化为原始记录，去掉复验与上传证据。详情缓存包含主分支 commit，避免删文件后仍复用旧详情。
5. 更新 testing.records 的原受控浏览器闭环，使用真实 fixture 断言及新仓库命令生成记录，验证主分支合入前后、重复导出、失败保留和删除；继续沿统一新包生成与正常停机，不访问安装目录或真实模型/PR。

## 最小验证安排

| 验收项 | 长期级别/组 | 执行安排与通过条件 |
| --- | --- | --- |
| GTR-A01 | L4 go.full owner 定向 | `go test ./internal/testmanagement ./cmd/test-feedback -run '^TestTR' -count=1`；格式/来源/完整性/重复导出反例通过 |
| GTR-A02 | L4 go.full owner 定向 | `go test ./internal/app -run '^TestTR' -count=1`；临时 Git 无数据库，主分支/坏文件/删除正确 |
| GTR-A03 | L4 go.full owner 定向、Web | `go test ./internal/product/httpapi ./internal/storage/sqlite ./internal/testmanagement ./tests/e2e/internal/driver -count=1`；相关 HTTP/schema/候选证据/实际模板脚本回归通过；`npm --prefix web test -- src/features/tests src/features/runs/plan-tests.test.tsx src/app/project-switch.test.tsx` 及 `npm --prefix web run build` |
| GTR-A04 | L4 testing.records controlled | `go run ./cmd/test-feedback -level=L4 -suite=testing -env=controlled -scenario=testing.records -out <全新绝对目录>`；原始脚本退出码、Git 文件、API/浏览器与停止/资源清理证据完整 |
| 常规回归 | L1、L2 | 修改完成后运行固定两个入口；各自 summary 和 managed 报告保留，不以脏源码报告生成固定 SHA 历史 |
| 文档 | 静态 | `python scripts/check_doc_links.py`、清单映射检查及 `git diff --check` |

先完成组件验证及 Web 构建，再串行执行 testing.records，避免共享源码构建输出竞争。各次输出用全新临时目录，失败不覆盖、不自动重试。人工 GTR-M01 后置。实际用例数、退出码、证据目录及未验边界写专属验收，不继承旧数据库方案通过事实。
