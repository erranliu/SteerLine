# 本机 Web 前端验收

专属普通开发任务：[Spec](../spec/web-frontend.spec.md)、[Plan](../spec/web-frontend.plan.md)。2026-10-14 用户直接授权实现，不分配 roadmap 编号。要求与命令映射归 Spec/Plan，本页维护实际结果。

| ID | 行为 / 证据要求 | 状态 |
| --- | --- | --- |
| WEB-A1 | 独立 npm 工程、TS/生产构建、真实静态嵌入及普通 Go 构建 | 待验 |
| WEB-A2 | 认证/一次性票据/Origin/CSRF/Host/固定端口/私有文件/静态边界；native 保持 | 待验 |
| WEB-A3 | 持久化先行、原 ID 对账/unknown/原信封补交、刷新/跨标签/实例隔离/终态和异常回执 | 待验 |
| WEB-A4 | 原 DTO 与动作映射、任务/DAG/执行/回答/Activity/final/PR/能力和离线 | 待验 |
| WEB-A5 | IME/Enter、草稿/晚到响应/分页隔离、安全正文、深链接/窄屏 | 待验 |
| WEB-A6 | 新独立 SteerlineTest simulate 包的真实浏览器闭环和进程清理 | 待验 |
| WEB-M1 | 用户观察桌面输入法、布局和原生查看窗口；自动验证后参与，窗口待安排 | 待人工，可后置 |

原始日志/截图写本次新临时目录或独立包 evidence，后续登记源码/构建、环境、命令、实际覆盖及限制。构建通过不代替实际服务接线；原型/fixture 数据不作为真实运行证明。
