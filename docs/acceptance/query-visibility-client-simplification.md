# 查询、可见性与独立客户端精简

2026-10-22，用户授权普通开发，按查询收口、可见性复用、独立写入分支删除的顺序实施。基线 `8e5babcfaa15d6fba436cb301a717cba0c5b2947`，结果来自该提交加验证时未提交的本次修改；提交后仍保留原报告的 dirty 标记，不改计干净固定 SHA 的资格。

## 范围与结果

| 自动项 | 实施与通过条件 | 结果 |
| --- | --- | --- |
| QVC-A1 | 任务材料与候选文档通过 `projectQueries` 查询；键包含 home/project/instance，手动刷新和项目级取消可命中，原 signal 传到 API，迟到响应不安装 | 已验收；初验两个文件 21 项通过，构建通过；最终参数化合并后随全 Web 复验 |
| QVC-A2 | 四个功能共用 `usePageVisible`，消息与费用共用精确查询取消；保留检查资源族取消、消息时间及费用完成条件 | 已验收；定向五文件 24 项通过，全 Web 最终 26 文件 / 176 项通过，构建通过 |
| QVC-A3 | 删除独立 `OpenCommands` 工厂及唯一消费者 `clientCommands/applicationStore/offlineRunnerDiscard`；正式服务仍注入 actor 命令端口，独立读取与 Decision 保留 | 已验收；五个 Go 包定向 120 项、保留读取/原请求三项均 passed/complete，零失败/skip；E2E driver 仅编译通过 |

可见性、停用、卸载和精确取消的通用断言归共享 hook 测试；候选选择切换与迟到响应由消息/费用共用的参数化组件集成测试验证。功能 owner 继续验证自身轮询、初始隐藏准入、协议身份、源码时间、费用快照与取消范围。查询作用域与刷新/取消共用一个参数化测试骨架，没有新增 E2E 场景或 L1 owner。

源码统计包含新增共享文件、删除文件及所有本次 Go/TS/TSX 修改，按 HEAD 原文与工作树文本逐文件比较，含空行、不含文档、依赖和生成产物。首次实施为生产代码减少 64 行、测试减少 11 行、合计减少 75 行；复审补回独有覆盖并清理残留后的最新统计为生产代码减少 69 行、测试增加 20 行、合计净减少 49 行。各轮逐文件统计分别保存于各自证据根的 `size-delta.json`，不为减少测试行数删去独有断言。

## 环境、命令与证据

Windows/amd64，Go 1.26.4，仓库内 `npm ci` 安装 lockfile 依赖。全部为隔离单元/集成或编译检查，不使用安装目录、真实模型或真实 PR。

原始证据根：`C:/Users/lmas1/AppData/Local/Temp/steerline-refactor-20261022-130704-153/`。各项日志和 Go summary 保存实际命令及结果。

```powershell
# QVC-A1：step1-web-final.log、step1-build-fixed2.log
npm --prefix web test -- src/shared/api/queries.test.ts src/features/runs/candidate-documents.test.tsx
npm --prefix web run build

# QVC-A2：step2-web-fixed.log；最终 web-final.log、build-final.log
npm --prefix web test -- src/shared/query-visibility.test.tsx src/features/runs/last-message.test.tsx src/features/runs/run-usage.test.tsx src/features/environment-checks/environment-checks.test.tsx src/features/tests/tests.test.tsx
npm --prefix web test
npm --prefix web run build

# QVC-A3：step3-go/summary.json、step3-retained-read/summary.json
go run ./cmd/test-feedback -out <新证据目录> -- ./internal/app ./internal/product/client ./internal/product/httpapi ./internal/taskrunner ./internal/storage/sqlite -run 'Test(NativeProject|CandidateRead|CM71|ProjectRuntime|ProjectLifecycle|RunnerDelivery|Client|Product|API|EnvironmentCheckHTTP|Task|.*Discard|.*Command)' -count=1
go run ./cmd/test-feedback -out <另一新证据目录> -- ./internal/app ./internal/product/client -run '^Test(FileQuerySourceReadsCombinedProjectView|WaitServiceCommandResolvesIdentityAndKeepsPolling|TM05OriginalRequest)$' -count=1
# step3-driver-build.log，仅编译，不能算 E2E 场景通过
go test -tags=e2e ./tests/e2e/internal/driver -run '^$'
```

首轮构建发现新增参数化测试的 TS 类型错误，修正后构建通过，原日志保留。可见性定向首轮发现测试管理的旧 `document.hidden` mock；初始隐藏断言已统一到共享测试，功能离线断言保留。

首次全 Web 运行还发现 Workspace 测试对项目维护返回错误形状的任务列表，产生渲染异常；未修改 HEAD 的独立归档副本运行同文件也有五项失败及五个未处理异常，见 `baseline-app-test.log`。首次实施临时隔离维护面板；下述复审已撤销该整体 mock，统一正确的 HTTP 响应夹具并恢复真实面板接线。连接、缓存、原信封及手动/自动对账断言保持。最终全 Web 零失败、无未处理异常。

## 2026-10-22 改动复审与修复

用户要求重新审计行为回归、测试独有覆盖与残留代码。逐项对照原测试、当前调用链及公开接口后，修正如下：

| 项目 | 问题与修复 | 最新验证 |
| --- | --- | --- |
| QVC-R1 组件独有覆盖 | 共享 hook 测试无法证明组件传入正确的候选查询键；恢复消息/费用真实组件的候选切换、原 signal 取消、迟到响应和新候选内容验证，共用一个参数化夹具；恢复消息与测试管理初始隐藏时禁查询的组件断言 | 全 Web 27 文件 / 179 项通过；共享生命周期及真实组件接线三项补验通过 |
| QVC-R2 Workspace 接线 | 撤销首次实施的维护面板整体 mock；共用真实 HTTP 路由响应与 snapshot 夹具，未知路由明确失败，保留真实面板渲染及查询白名单；复用挂载骨架减少重复准备 | Workspace 六项及全 Web 通过，构建通过 |
| QVC-R3 删除链残留 | `TaskCommands.Discard`、`DiscardTask` 回退、runtime 不支持方法，以及旧开库工厂使用的 `mutation` 参数仍然残留；移除后 discard 只走显式 actor 端口，命令工厂不再传无消费者的布尔值；既有资源测试补验显式端口、缺失端口和关闭后拒绝 | 三个 Go owner 包完整运行 111 项、app 定向 11 项均 passed/complete，零失败/skip；E2E driver 编译通过；残留符号检索零引用 |

本轮证据根：`C:/Users/lmas1/AppData/Local/Temp/steerline-refactor-audit-20261022-144538-807/`。

```powershell
# web-final.log、coverage-owners-final.log、build-audited.log
npm --prefix web test
npm --prefix web test -- src/shared/query-visibility.test.tsx src/features/runs/candidate-selection.test.tsx
npm --prefix web run build
# go-consumers-final/summary.json、go-app/summary.json
go run ./cmd/test-feedback -out <新证据目录> -- ./internal/product/client ./internal/product/httpapi ./internal/taskrunner -count=1
go run ./cmd/test-feedback -out <另一新证据目录> -- ./internal/app -run '^Test(FileQuerySourceReadsCombinedProjectView|ProjectRuntime.*|ProjectLifecycle.*|NativeProject.*|.*Discard.*|.*Command.*)$' -count=1
go test -tags=e2e ./tests/e2e/internal/driver -run '^$'
```

本轮没有发现已验证范围内的业务行为回归；读取、候选/身份隔离、原请求、actor 命令以及生命周期边界按上述范围复验。未执行的真实流程仍不计通过。

未运行 Full、完整 L1、L2/L3/L4 或浏览器现场；本次没有改变后端协议、原生执行及业务事务，不增加这些流程验收。没有新增人工验收项。
