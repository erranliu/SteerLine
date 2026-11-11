# 自动派发开关验收

受测源码基于提交 7735f44d4265452e9737c73b64ea713db44f7bbd，包含本次工作树改动。新项目默认关闭自动派发，符合资格的任务可直接手动启动。开启后按现有项目、环境、依赖和执行槽规则派发。关闭后停止后续自动派发，已启动的执行继续。

自动派发模式是唯一的持久化设置。Web、CLI、HTTP 和任务启动准入移除原暂停状态及命令，沿现有模式命令保存版本和原请求回执。项目数据库使用当前格式 46。安装环境和旧库未操作。

## 验证

报告根为系统临时目录中的 `steerline-dispatch-0c02514de2a14efb85e07c4e9f491ed1`，另列报告给出各自的目录名。
串行复验的报告根为 `steerline-dispatch-check-8ae521f74a394e49b9587d283686f32b`。

| 执行 | 结果与原报告 |
| --- | --- |
| `go run ./cmd/test-feedback -level=L1` | 通过，`L1/summary.json`。 |
| `go run ./cmd/test-feedback -level=L2` | 通过，`L2/summary.json`。默认关闭自动派发时直接手动启动，贯通交付和后继释放。 |
| `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.deployment` | 通过，`go.deployment/summary.json`。 |
| `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.http` | 串行复验通过，`go.http/summary.json`。 |
| `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.storage` | 串行复验通过，`go.storage/summary.json`。 |
| `go run ./cmd/test-feedback -level=L3 -suite=runtime -env=controlled -scenario runtime.service-lifecycle,runtime.async-activities` | 服务生命周期场景通过。活动场景首轮超时，组合报告为 `runtime-L3/summary.json`。 |
| `go run ./cmd/test-feedback -level=L3 -suite=runtime -env=controlled -scenario runtime.async-activities` | 单独复验通过并完成清理，`steerline-dispatch-async-clean-c7a5dedccee44e71a98199fe66f308e2/summary.json`。 |
| `go run ./cmd/test-feedback -level=L4 -suite=runtime -env=controlled -scenario runtime.service-write-isolation` | 通过，串行复验报告根的 `runtime-L4/summary.json`。 |
| `go run ./cmd/test-feedback -level=L4 -suite=web -scenario web.unit` | 457 项通过，`steerline-dispatch-web-recheck-96f24be3e0fa496f8f4f65df9346328c/summary.json`。 |
| `npm --prefix web run build` 与 `npm --prefix web run test:e2e -- configuration-navigation.spec.ts` | 生产构建及四项 Edge 场景通过。浏览器证据在 `steerline-dispatch-web-a1fc059cd3314e4782828f285e846244/browser/`。 |

首轮 Web 全集中的操作回执断言改用可判定元素缺席的查询，项目切换测试定向复验和全集复验通过。首轮报告保存在 `steerline-dispatch-web-a1fc059cd3314e4782828f285e846244/unit/summary.json`。

首轮 HTTP 跨动作冲突夹具误用了相同信封，已改为不同有效动作。首轮存储专项的活动分页关联查询断言失败，同一测试串行复验通过。首轮 Go 专项报告保存在初始报告根下的对应子目录。

活动场景首轮在浏览器等待窗口内未收到首次启动回执。第一次单独复验通过浏览器断言，诊断读连接占用测试库，导致清理失败，报告为 `steerline-dispatch-async-1b7f57752f3945fd9948ed958c457144/summary.json`。第二次单独复验在全新现场验证手动启动、新增候选和正常清理。
