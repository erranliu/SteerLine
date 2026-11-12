# Web UI 恢复可见时刷新

2026-11-12，受测版本为 `9cefc59858e3f37d65397fc435f2d71638a3861b` 加本次未提交的 Web 修改。

页面隐藏时暂停周期查询。页面恢复可见时，当前启用的动态查询立即重取，即使缓存仍在有效期内。活动列表只刷新首屏观察，已加载的分页及按需读取的交互快照沿用原有刷新规则。

| 验证 | 结果与原报告 |
| --- | --- |
| `go run ./cmd/test-feedback -level=L1 -out <L1目录>` | 通过。`C:/SteerlineDebug/wt-e6c3-foreground/test-results/L1/6ff8aef8f36e4710b3d85b6f7d5f175f/summary.json` |
| `go run ./cmd/test-feedback -level=L2 -out <L2目录>` | 受控交付冒烟通过，清理完成。`C:/SteerlineDebug/wt-e6c3-foreground/test-results/L2/b4c21931578343569c6c13ceaf3b4fa2/summary.json` |
| `go run ./cmd/test-feedback -level=L4 -suite=web -scenario web.unit -out <L4目录>` | 53 文件、486 项通过。`C:/SteerlineDebug/wt-e6c3-foreground/test-results/L4/e2326ef96ec349719d229af685e72a2c/summary.json` |
| `npm.cmd --prefix web run build` | TypeScript 检查与 Vite 生产构建通过。 |

回归用例模拟页面隐藏及恢复可见，验证后台经过三个轮询周期仍无请求，恢复可见后在推进轮询计时前即发起请求。活动页用生产查询配置验证首屏更新与分页快照并存。首次沙箱运行的报告重命名被拒绝，首次前端全量运行的活动深链接用例超时。深链接用例单独复跑及随后串行全量均通过。

浏览器实机切换与安装环境部署未执行。
