# 固定提示词维护

SteerLine 自身的固定行为说明统一放在 [internal/prompts](../internal/prompts/)，通过同包的小函数嵌入、渲染和组合。原业务 owner 校验事实、选择步骤及材料，适配点只传原正文和事实。不得在业务 Go 字符串、诊断参数或渲染后的正文中补固定规则。

Runner 正文、用户覆盖/原回答、测试安排、规范、schema、报告和正式 writer 继续使用各自原来源。仅固定模板统一 UTF-8/LF；用户文字和资产不得被统一 trim 或改换行。无同源 writer 的项目仍从 [正式资产](../tools/progress_assets.go)获得完整源码。

## 修改与本地检查

1. 修改对应 Markdown；确需改变组合方式时，在 prompts 及原适配点修改，保持阶段、来源和原请求冻结约束。
2. 运行 `go test ./internal/prompts -count=1 -v`，核对 `testdata/*.golden.md` 的完整正文差异和 UTF-8 字节数；再按[测试指南](../SteerlineFlow/testing.md)运行正式 L1/L2 与受影响的 owner/L3。黄金样本没有自动接受/更新开关，修改期望值需显式审阅。
3. 在 PR 写清业务依据、实际正文差异、字节数前后值和验证。增长说明替代了哪些旧规则；新增且无替代时说明原因。缩短、等长替换和测试/检查配置变化同样需要独立审核。

模板和实际请求样本是可读的 Git 文件。内联 writer 的样本使用测试占位符引用正式资产，不再复制一份 PowerShell 源码；比较前扩展为实际完整正文，运行时仍完整发送原资产。包含用户 CRLF 的黄金样本通过 [.gitattributes](../.gitattributes)保留字节，不能批量归一化样本。

[源码检查](../internal/testkit/boundary/prompts.go)只限制模板嵌入所有者、指令字段写入点和适配点直接内联/追加语法，包含非当前 build tag 文件。不追踪任意函数的语义；间接夹带行为要求靠实际请求断言和代码审核发现。

## GitHub 原生接入与未验边界

2026-10-28 只读查询 `erranliu/SteerLine`：Actions 工作流列表为 0，main 分支保护返回 `404 Branch not protected`。本次提供普通 [Actions job](../.github/workflows/prompt-governance.yml)，检查名称为 `prompt-governance`，复用完整 L1。所有 PR 均触发，无路径过滤；仅只读 contents 权限，checkout 不保留凭据。配置使用官方 [checkout](https://github.com/actions/checkout) 和 [setup-go](https://github.com/actions/setup-go)；Go 版本取 go.mod。

仓库文件交付不等于启用分支保护。获得外部配置范围授权后，由有权限的维护者在 GitHub 原生设置中完成并按 [PG-E01](../SteerlineFlow/testing/acceptance/prompt-governance.md)取证：

- workflow 首次在目标版本运行成功后，设其 `prompt-governance` 状态为 main 必需检查，要求 PR 基线最新或使用已核验的合并队列。
- 要求至少一位具有资格且独立于作者的审核者批准，开启新提交撤销过期批准。审核者须能覆盖模板、组装/接线、正式资产、黄金样本和 CI/源码守卫的变化；不接受仅更新样本或检查程序就免审。
- 可用原生代码所有者规则指定上述路径；尚未确定合格独立审核者时不虚构 CODEOWNERS。无法按路径控制时采用仓库统一独立审批。
- 禁止普通角色直接推送或绕过规则；实际配置与账号资格以平台核验为准。专属测试 PR 验证缺状态/缺审批拒绝、新提交撤销旧批准、最新合法提交可合并及资源清理。

本次未修改 GitHub 保护、发送 PR 或邀请审核者，也未运行远端 job。管理员主动关闭保护超出保证范围；PG-E01 未满足前仅声明本地检测及可审查接入配置已交付。
