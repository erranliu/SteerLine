# 复制项目初始化与接入提示词

2026-11-13，受测基线为 `e833ea5e40d0d7698bfefd7780160a0ab3d31930` 加工作树差异。

「添加已初始化项目」页在表单前依次提供「复制项目初始化提示词」和「复制项目接入提示词」。第一段让目标项目的 Agent 阅读指定的接入指南并完成项目端初始化。材料进入目标分支后，第二段引导 Agent 核验并接入当前安装实例，由用户确认添加。两个按钮都支持在剪贴板写入失败时手动复制完整正文。

## 正文差异与独立审核

以下链接提供改动后的完整正文。模板字节数含尾换行，渲染正文不含尾换行，接入样本使用自定义 Windows 安装目录。

| 正文 | 模板 UTF-8 字节 | 渲染 UTF-8 字节 | 实际变更 |
| --- | --- | --- | --- |
| 初始化 | 0 → 761 | 0 → 760 | 新增用户指定的 HEAD 指南链接、项目端准备和自检步骤。[模板](../../../internal/prompts/project-initialization.md) · [渲染全文](../../../internal/prompts/testdata/project-initialization.golden.md) |
| 接入 | 1,441 → 1,088 | 1,456 → 1,103 | 将读取指南、准备和自检交给第一步，保留实际安装目录、目标分支核验、用户确认和按指定目录重启。[模板](../../../internal/prompts/project-registration.md) · [渲染全文](../../../internal/prompts/testdata/project-registration.golden.md) |

应用目录通过 `internal/prompts.ProjectInitialization` 和 `internal/prompts.ProjectRegistration` 渲染正文，Web 经已认证的 `GET /api/v2/application/projects/registration/prompt` 读取并复制。安装目录取自服务实例，浏览器无需提供安装路径或先填写项目表单。

独立只读审核发现首个项目的导航缺少零项目页面入口，修订后复核通过。审核者独立渲染并核对两份全文和 UTF-8 字节数。

## 验证

- 提示词、应用目录和 Web 登记定向 Go 测试通过。覆盖指定指南、实际安装路径、无项目时生成、浏览器会话认证和两段正文转发。最终接入正文的入口修订补跑 `go test ./internal/prompts -count=1 -v`。
- `npm --prefix web test -- src/app/project-switch.test.tsx` 通过 23 项，覆盖两个按钮的复制内容、手动复制、重试成功和原登记流程。
- `npm --prefix web run build` 通过。
- Edge 使用生产前端和隔离 HTTP 夹具，在 1440px 和 390px 验证两个按钮的顺序、实际剪贴板和手动复制内容、页面无横向溢出、全程没有写请求。Windows 剪贴板将 LF 转为 CRLF，换行归一后正文与黄金样本一致。截图位于 `C:/Users/lmas1/AppData/Local/Temp/steerline-74df-project-prompts-browser-8718fcb134eb46ecb73ae25b98d04983/`。
- L1、L2 和 HTTP 专项 `go.http` 通过，L2 确认清理完成。报告根为 `C:/Users/lmas1/AppData/Local/Temp/steerline-74df-project-prompts-4a07d70f2dd64a5f83e0a7070577c29c/`，分别见 `L1/summary.json`、`L2/summary.json` 和 `go.http/summary.json`。零项目入口文案的最终修订用提示词专项和 Edge 检查验证。

真实项目 Agent 操作和安装实例部署未执行。

## Agent 完成项目接入

2026-11-13，变更基线为 `97590c296b3421b2e94661b03bd6200d7653cbf2` 加工作树差异。

为使 Agent 完成添加，将「请在核验后交给我确认添加」替换为「点击『核验目标分支』，确认页面显示的提交与目标提交一致，然后直接点击『确认添加』并检查保存结果」。保存后仍按提示词中的安装目录重启，现要求确认项目已加载并报告目标提交。页面无法操作时原先仅回传参数，现要求说明已完成的步骤、具体阻塞和所需帮助。[模板](../../../internal/prompts/project-registration.md) · [渲染全文](../../../internal/prompts/testdata/project-registration.golden.md)

模板 UTF-8 字节数为 1,088 → 1,229，含尾换行。渲染样本为 1,103 → 1,244，不含尾换行。独立只读审核通过，核对了实际全文、前后字节数、逐字节渲染结果和 Web 核验、确认及重启流程。

- `go test ./internal/prompts -count=1 -v`、`go test ./internal/app -run '^TestProjectRegistration' -count=1` 和 `npm --prefix web test -- src/app/project-switch.test.tsx` 通过。前端用例通过 23 项。
- `go run ./cmd/test-feedback -level=L1` 和 `-level=L2` 分别以 `-out` 指定独立目录后通过。L2 报告确认清理完成。报告位于 `C:/SteerlineDebug/wt-4048-registration/test-results/` 下的 `L1/72dc01f5cee045a9afb0a37140ade305/summary.json` 和 `L2/7b0e339d093c412a9787a51cba3e09ef/summary.json`。沙箱内的初次运行遇到临时仓库写入和报告重命名权限错误，复测在独立目录完成。
- 真实 Agent 接入和安装实例部署未执行。
