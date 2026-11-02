# 开发方案权限界面与构造精简

2026-11-02。对应[Spec](../../spec/runtime-project-environment-decoupling.spec.md) §3.1、[Plan](../../plans/runtime-project-environment-decoupling.plan.md) P2。用户确认实施并提 PR；受测基线 `fd84478926bea32fbfa687ece56e931470924faf` 的 dirty 工作树。

开发契约固定 full_access，Web 删除权限下拉及“Runner 默认/沙箱”选项，在原更多设置位置显示固定说明；删除仅供该下拉使用的 patchSettings。链路切换公共路径及能力投影公共初始化各写权限一次，新建方案仍在原构造入口显式设置。用户明确切换至开发工作流时设置 full_access，内容方案的普通改名不改 read_only。没有新增权限抽象、配置字段或兼容路径；Runner 必填字段、真实保存校验和生成/对话只读契约保持。

证据根 `C:/SteerlineDebug/wt-68cd-simplify-access-mode/`；每次入口使用独立目录，完整命令、源码和终态在原报告。

| 实际选择 | 结果 | 相对证据路径 |
| --- | --- | --- |
| 完整 Web 服务及 runnerclient owner | 166项通过、0失败/0跳过，41.5秒；覆盖开发能力投影、严格保存及原只读生成构造 | `diagnostics/contracts/summary.json` |
| go.http | complete=true/passed=true，104项、0失败/0跳过，执行53.8秒 | `test-results/L4/http/summary.json` |
| 最终 web.unit | complete=true/passed=true，31文件234项、0失败/0跳过，执行27.4秒；覆盖固定说明和删除选项、只读改名保留以及显式切换开发的提交参数 | `test-results/L4/web-final/summary.json` |
| 固定 L1 | complete=true/passed=true，执行53.0秒 | `test-results/L1/initial/summary.json` |
| 固定 L2 | passed=true/cleanup_complete=true，执行202.5秒 | `test-results/L2/initial/summary.json` |
| Web 构建与浏览器 | npm build 通过；configuration-navigation.spec.ts 两场景通过，30.2秒；配置导航、固定权限说明、无过时选项及375/900宽度均覆盖 | `diagnostics/browser/`，profiles-desktop.png 已查看，trace保留 |

首轮 `test-results/L4/web/` 也通过；新增只读转换用例补充等待原保存成功后，以 web-final 独立目录重验完整前端集。没有断言失败、意外跳过或放宽测试条件。文档本地链接与 diff 空白检查另行核对。

本轮只改编辑器及配置能力试样，不改持久 schema、后端权限默认、项目环境依据、原生程序/helper来源校验、实际 Runner 调用接线、提示词或模型材料。未新增安装方案、修改安装配置、重启服务或执行真实 Run；不把受控验证当作部署生效或实际项目检测就绪。

最终验证后独立只读审核通过：权限只在新建或明确切换开发链路/工作流时写入，只读改名仍保留；后端、程序/helper边界与生成契约未放宽。已核对166/104/234项、L1/L2和清理、浏览器trace及文档声明，无确认问题。
