# Plan：环境检测使用 Windows 自带 PowerShell

[Spec](../spec/environment-powershell-baseline.spec.md)。直接实施，不调度 roadmap。沿用证据根 `C:/SteerlineDebug/wt-5069-local-orca-environment/`，每次独立新输出，不部署。

1. PSB-A01：同步固定 generation/execution/testing 材料与项目镜像；`.ps1` manifest 使用 powershell.exe。更新实际调用及受影响夹具，解释器由文档固定，不按机器临时回退。
2. PSB-A02：统一替换 PS7 JSON/摘要/文件 API；共享 writer 仅解析协议规定的严格 JSON 布尔对象并保留 Ordinal 键、锁、刷新和原子替换。308 只调整正式 L1/L2 与受管协议必经调用，保留完整选择与通过标准。
3. 实施中验证 PSB-A02/A03：真实 5.1 子进程运行三仓 TTY/失败报告回归、SteerlineTest L2 原报告绑定、根仓进度 writer 严格 JSON/Unicode/并发互操作、308 报告/进度协议。使用不含 pwsh 的 PATH；逐项核对非零执行、拒绝项原始字节与清理，不将夹具等同 Unity 业务验收。
4. 独立代码审核后提交冻结源码，再以干净固定输入验证 PSB-A01–A03：根仓正式 L1/L2、定向 environmentadaptation/prompts/runtimeprogress，以及 controlled L3 `testing.project-environment-readiness`；SteerlineTest 正式 L1/L2；308 通过原正式 L1/L2 执行，缺业务依赖或失败如实保留。文档链接、清单一致性、同源正文与 diff 检查。
5. PSB-A04：核实当前 Orca runtime、旧测试驱动已结束及原未知资源边界；固定干净根仓/项目提交、已有独立 Runner/profile、新输出，运行正式 live L4 `testing.project-environment-local`。原 Run 由正式入口停止并封存，不向原 worker 补发输入、不借旧结果或直接关闭未知 worker。
6. 独立复审实际正文、字节、代码、测试及本机原生证据，修正确认问题。专属验收登记每轮输入/报告/失败与未验边界；提交验收记录并更新现有 PR，不合并或部署。

原生续验发现 PS5 将空 JSON 数组作为单个管道对象保留，导致发布脚本误取空 PR URL。PSB-A02/A03 补充：正规化原 GitHub 列表，不改发布顺序；三仓 `python scripts/test-environment-publication.py` 在真实 PS5、无 pwsh PATH 下执行原 provider 函数及原发布代码块，验证零条仅创建一次、单条复用、多条在创建 PR 前拒绝、base/head/SHA 错误及查询失败保留 unknown。308 先验证原 Gitea 路径，不为同形而改代码。回归及独立审查后，以新固定提交重跑正式本机 L4；原失败、停止与补充资源清理单独登记。

第二次续验中 Agent 因固定脚本缺少提交前暂存 diff 核验而未执行发布。PSB-A03 补充三仓相同的提交前校验：非空 diff、唯一原探针路径、暂存正文与本轮标识精确一致、diff --check，完成后才 commit；不新增提示词例外。上述同一回归入口运行原探针提交代码块和真实隔离 Git，验证成功仅提交探针，以及额外暂存文件、空索引、正文篡改、检查失败均在 commit 前拒绝。此前未发布的原活动正常停止并保留原失败，不借重跑覆盖。

固定 generation 材料及三仓镜像同步声明这项提交前检查，避免后续生成丢失守卫；项目文档说明实际脚本检查。独立审核实际正文与字节变化后冻结，不引入行为规则的 inline 兜底。
