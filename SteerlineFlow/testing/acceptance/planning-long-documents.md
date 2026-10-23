# 规划原型长期文档交互验收

仅归属 PR #363，源码 `5612f97d6754b98e79b5be409ba688e204cd8832` 中的[单文件原型](../../../docs/product-design/prototypes/planning-ui/index.html)及[说明](../../../docs/product-design/prototypes/planning-ui/README.md)。本项只验页面演示，不涉及真实文档写入、模型、PR、Task 或 roadmap 执行。

2026-10-20，用户授权补验。Windows / headless Edge / 本仓 Playwright，实际打开本地 HTML。自动五项均通过：流程与项目文档分组可见、流程文档有规范维护入口、保存演示规范保留原正文、待合入预览含编辑内容、项目文档不显示流程规范维护按钮；`pageerror=[]`。

原始自动脚本、五项断言结果和截图：`C:/Users/lmas1/AppData/Local/Temp/steerline-merged-acceptance-20261020-125058/planning-prototype.cjs`、同目录 `planning-prototype.json` / `planning-prototype.png`。复现：`node <上述脚本> <仓库绝对路径> <证据输出目录>`，依赖本仓 `web/node_modules/@playwright/test`，输出只写证据目录。未新增长期浏览器套件，脚本不写项目文件。

人工可后置，当前未验；参与人为用户，窗口待安排，只影响原型体验签收：

1. 浏览器打开上述 HTML，左侧选“长期文档”，检查两组目录是否易找、长路径和正文是否易读。
2. 选流程规范，分别点“查看最新规范”“维护规范”，编辑一小段并保存；在“正文快照”与“待合入更新预览”间切换，确认文案能清楚表达保存的是待合入草稿。
3. 选项目架构文档，确认目录层次、文档标题/路径、版本/覆盖说明及维护记录易读。
4. 从草稿转正的生成规范入口进入，再返回转正页，确认定位自然且无误导为真实后台操作的文案。自动检查未覆盖此主观往返体验。

本项不修改或执行 roadmap，不把演示的“合入/运行/生成”计为产品能力通过。结果登记参与时间、浏览器/源码版本、实际步骤及截图。
