# Spec：本机 Web 前端与 DAG / 执行闭环

> 本 Spec 保留 Web 首次实现时的原始交付范围；其中“保留 TUI”是当时条件，已由后续[TUI 退役任务](tui-retirement.spec.md)取代。当前只有 Web 是交互前端。

用户于 2026-10-14 直接授权本聊天生成 Spec/Plan 并实现；普通开发，不分配 Task ID，不加入 roadmap。依据[已确认技术方案](../product-design/web-frontend.md)、[原型](../product-design/prototypes/planning-ui/README.md)及[现有 HTTP 契约](../product-design/local-client-protocol.md)。配套[Plan](web-frontend.plan.md)、[专属验收](../acceptance/web-frontend.md)。

## 目标与范围

在相对独立的 `web/` 项目中实现 React/TypeScript/Vite SPA，使用 Tailwind/shadcn 基础控件、React Router、TanStack Query 与 React Flow。前端依赖、构建、测试、浏览器 HTTP 服务和静态资源都归该路径；Go 应用只负责配置、启动/关闭及 CLI 打开入口。复用现有 HTTP 客户端和 Go 业务权威，不复制任务、候选或回执规则。

首期本机浏览器真实接入，包含 DAG/列表、任务详情/依赖、执行候选切换、近期 Activity、完整 final、原问题回答、现有执行方案选择与首次/并行候选启动、自动派发切换、停止候选参与及原生会话查看、PR/交付事实展示。停止参与沿[现有契约](candidate-stop.spec.md)，不停止 worker/会话或清理资源，不按 Task 状态推导参与资格，也不能停止最后一个参与候选。保持原型深色布局与青绿色强调色，提供窄窗口可用布局、可见焦点、中文多行输入和明确错误/空状态。缺少真实能力显示原因，不展示模拟业务数据。

规划 Agent、草稿转正、验收/测试产品管理、版本/Bug 管理、远程浏览器接入、SSE/工具过程正文及浏览器停服核实不在本批。保留 TUI；关闭标签页不影响服务/worker。

## 浏览器服务与认证

项目配置新增可空 `web_address`。非空必须为固定的 `127.0.0.1:<1..65535>`；空值不启用 Web。地址冲突启动失败，不随机换端口；不同项目/独立调试包须选择不同地址。服务重启保留配置地址，以保持浏览器 Origin 和 IndexedDB。原 native API 仍按已有随机端口、Bearer 和 Origin 拒绝规则运行。

Web 服务和 native API 同属 Go 服务生命周期。浏览器服务通过已有 `httpclient` 通用查询/命令转接同一 native API，必要性是浏览器不能读取受保护凭证且 native API 明确拒绝浏览器 Origin。该转接只承担认证及协议映射，不维护业务库/调度器，不解析或改写原命令身份/版本。

`steerline web -config <项目配置>` 从当前用户私有的 `<application_home>/ui/web/<project_id>/discovery.json` 取得原实例打开资格，向固定 loopback 地址申请 60 秒有效的一次性票据，再打开浏览器；`-print-url` 用于受控调试。票据只在 URL fragment，页面先移除 fragment 再兑换会话，原生 Bearer 不进入浏览器。重复/过期票据拒绝。Web discovery 原子写入且沿用当前用户 ACL；CLI 与原 native discovery 核对项目、实例和地址，不启动替代服务。不在 native run_dir 或 local-api 中新增目录/格式。浏览器会话保存在服务内存，绑定本实例、有效期 12 小时；重启需要重新打开授权。

所有请求严格核验 loopback 与 Host；跨站 Fetch Metadata 拒绝。会话 cookie 为 HttpOnly、SameSite=Strict，名字按项目/端口隔离；写请求须匹配精确 Origin、JSON Content-Type 与会话 CSRF token。认证、API 和 HTML 禁缓存，页面设置 CSP/Referrer-Policy。API 不放开 CORS，不接受外部指定 upstream。静态路径不允许遍历，缺失资源/API 不能回退 HTML。

## 页面、目标与命令

任务/DAG 使用已有有界 DTO，分页显式显示；更换数据库/图 revision 或游标失效时刷新，不能拼接不同快照。DAG 不允许前端编辑业务依赖。选中任务和候选在 URL 中定位，查询 key 带项目/实例及精确执行身份，晚到结果不覆盖其他目标。

业务命令必须先持久化完整不可变信封到 IndexedDB，失败不发送。查询和 mutation 禁用隐式重试；网络失败/unknown 保留请求与草稿。刷新/重连后只查询原 request_id；仅权威 command_not_found 且仍为原实例才补交原信封。不同内容不覆盖同 ID；每个原请求跨标签页使用同一发送锁；终态结果不被晚到 unknown 覆盖。回执必须匹配原项目、实例、action、target、版本及 original 信封。pending/accepted、applied、rejected 与 unknown 分别展示。

离线或身份不一致时禁写，保留最后观察时间与输入。草稿按项目/实例/Run/Execution/Attempt/Decision/版本绑定，候选切换不转投；IME 组词和 Enter 不触发发送，显式按钮提交。回答前核验原 eligibility，服务仍最终校验版本/资格；可选反馈与真正必答问题区分。

final/Activity 仅来自正式接口。超过 final 有界响应时通过 conversation/body 按原 revision/SHA/offset 读取，显式分页及正文完整性，不把 partial 当作完整。PR 链接仅使用安全 HTTP(S) URL；Agent 文本不执行 HTML。原生查看绑定原 run/Execution/Attempt/session/generation，unknown 不重复打开。

## 自动验收

| ID | 通过条件 |
| --- | --- |
| WEB-A1 | `web/` 独立 npm 工程可安装、类型检查、构建、测试；带 Web 构建的 Go 包嵌入真实静态产物；普通 Go 构建不依赖 Node/产物，启用未构建 Web 明确报错 |
| WEB-A2 | 非 loopback/Host/跨站/缺会话/错误 CSRF 拒绝；票据过期与重用拒绝；私有 discovery、静态边界和稳定端口冲突验证；native Origin 策略不放宽 |
| WEB-A3 | 先保存再发送；原 ID 对账、unknown 不重发、权威缺失补原请求、实例变化不重定向、刷新/多标签锁及终态保护、异常回执匹配验证 |
| WEB-A4 | 真实服务任务/DAG/候选/Activity/final/PR 展示；首次与并行启动、派发控制、停止、回答及原生查看映射原协议，能力/阻断/离线正确表达 |
| WEB-A5 | 中文 IME/Enter 不误发；不同候选/问题草稿隔离；晚到响应/失效分页不混目标；正文与恶意文本安全渲染，深链接/窄屏可用 |
| WEB-A6 | 新独立 SteerlineTest simulate 包运行真实 Go/Web/浏览器链路：授权、启动、回答、final、依赖/交付、刷新恢复和离线；退出浏览器不停止服务；证据与清理记录齐全 |

WEB-M1：用户在自动验证后观察桌面输入法、实际布局与原生查看窗口，窗口待安排；该人工体验项可后置，不阻塞无关开发，不将其计作自动通过。
