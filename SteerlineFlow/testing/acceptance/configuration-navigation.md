# 配置入口与导航统一

日期：2026-10-23，普通开发，按本聊天确认方案实施。基线 `a21b64f1f120b9eb3d128af467770e1d6e5eee26`，以下为该 worktree 未提交修改的验证，不登记成干净提交测试历史。

## 确认范围与实现

- “全局执行配置”改称“执行方案配置”，保留全局执行方案、模型、默认方案管理。
- “执行设置”改称“配置”，不限定项目专属设置；移除重复的执行方案列表及加载说明，保留环境检查与项目信息。
- 项目维护的生成并发设置移至“配置”，明确所有项目共享；仍保存 execution.json 的 `max_generate_concurrency`，默认 5、正整数校验、条件写入和重启生效均保持。共用原保存后核对逻辑，不覆盖执行方案及高级字段。
- 两页复用 AppShell 的侧栏、顶栏、页脚和项目页配色。项目导航、配置导航和收起后的图标保留可访问名称。
- 返回原项目、页面、查询和片段；刷新后可恢复本标签页记录，直达采用有效最近项目。失效项目不作为返回目标，无有效项目提供项目选择。浏览器偏好存储不可用时，当前标签页的内存记录仍保留返回位置。
- 无项目也可访问 `/settings` 和 `/settings/execution-configs`。新增 `/settings` SPA 路由；服务 API、共享生成队列及 Runner 行为不变。
- 执行方案和生成并发编辑均提示未保存离开，覆盖链接、项目切换、浏览器后退与刷新。改用 React Router data router 支持真实历史阻止。

## 自动验证

本次原始证据根：`C:/SteerlineDebug/wt-7d99-config/`。所有输出独立保留，没有使用或部署安装 home。

| 范围 | 命令与结果 | 原始证据 |
| --- | --- | --- |
| L1 | `go run ./cmd/test-feedback -level=L1 -out <独立目录>`，passed=true | `test-results/L1/20261023T070237786Z-b2631833c7f34efc8e5ea818dfc23956/summary.json` |
| L2 | `go run ./cmd/test-feedback -level=L2 -out <独立目录>`，passed=true | `test-results/L2/20261023T070507726Z-27389d295b614ca39c63642c2fe163bb/summary.json` |
| 构建 | `npm --prefix web run build`，TypeScript/Vite 通过 | `diagnostics/build-final-20261023T071446552Z.log` |
| 前端全量 | `npm --prefix web test -- --maxWorkers=2`，27 文件、186 项通过 | `diagnostics/client-final-20261023T071200561Z.log` |
| 本次 HTTP 路由 | `go run ./cmd/test-feedback -out <独立目录> -- ./web/server -run='Test(WebExecutionConfigRoutes\|WebOriginHostAndStaticBoundaries)$' -count=1`，2 项通过，含新增 `/settings` | `diagnostics/config-server-20261023T071200612Z-60699b5487d746bc91e9095e95a81902/summary.json` |
| UI 浏览器专项 | `npm --prefix web run test:e2e -- configuration-navigation.spec.ts`，Edge headless，1 项通过，零 pageerror | `diagnostics/browser-20261023T071200639Z-7f3a9e9763b04e17bbcc155a7dac8d80/`，含 trace、1440/900/375 宽截图 |

前端覆盖原项目/页面/query/hash返回、直达最近项目、失效项目回退、无项目设置、偏好存储失败、链接与历史后退未保存保护；生成并发覆盖校验、保留方案和高级值、412 保留输入、响应丢失后只读核对、冻结包只读。浏览器专项实际运行构建后的客户端，API 全部受控拦截，不计真实服务、原生目录探测或 Runner 验收；本次截图已检查布局，无横向溢出。

文档检查：`python scripts/check_doc_links.py` 检查 569 份 Markdown、3576 条本地导航链接，0 错误；`git diff --check` 通过。测试目录一致性两个测试补验通过，证据 `diagnostics/catalogue-20261023T071524903Z-e86ad6c69075479d997478dcac06ec9a/summary.json`。

## 原失败与补验边界

- 测试目录一致性首轮 `diagnostics/catalogue-20261023T071450911Z-073543df1ee54fd79ee8c533e79a7308/summary.json`：L4 表格内新增的 npm 独立命令被检查器当作机器测试 ID，1 通过、1 失败。改用表外正文说明独立命令，保留机器 ID 表格约定，补验两个测试通过，原失败不改标。

- `diagnostics/web-20261023T070551451Z-33984104ed884825976c6663267c498b/build.log`：首轮构建发现新增测试使用不支持的 Testing Library `exact` 参数及 unknown 字段访问；修正测试类型后构建通过。
- `diagnostics/web-20261023T070705141Z-e909355b81394f80afcef357c402a133/client-tests.log`：默认并行前端运行 184 通过、2 失败（answer 原生多问题提交、tests 原始 Git 记录展示）。原失败保留；这两个文件单 worker 10 项通过，证据 `diagnostics/isolated-client-20261023T070802215Z.log`；完整套件限制两个 worker 后 186 项通过，不把默认并行原运行改标成功。
- 首轮浏览器 `diagnostics/browser-20261023T070802156Z-2e995fbce1904fd5ac962ab0c37d9615/` 在 900 宽侧栏收起后，链接的隐藏文字不能提供可访问名称，调用等待期间主动结束。补上独立 aria-label/title 后新浏览器运行通过；原截图与 trace 保留。
- HTTP owner 全集 `diagnostics/server-20261023T070802201Z-ccb7b7124de148a38b9f694a9dd02a86/summary.json`：102 项中 101 通过、1 失败。`TestTRQueryBrowserRoutesAndReadOnlyBoundary` 请求旧 `/tests/records/<32位ID>` 返回 404，与本次 `/settings` SPA 改动无关。通过 Go overlay 使用 **HEAD 原版 server.go** 和未修改的 server_test.go，定向重现同样失败，证据 `diagnostics/baseline-http-20261023T071014-b5920fce63d84225bee3c5084bb6045f/result/summary.json`、`overlay.json`、`server-baseline.go`。本次未恢复已退役测试记录接口，HTTP 全集仍记录为未通过。

仅完成源码及自动 UI/路由验证，未部署安装版，未运行真实模型或原生探测，不改变既有维护/执行方案专项的人工签收状态。
