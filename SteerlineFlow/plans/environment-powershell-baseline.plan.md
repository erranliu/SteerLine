# Plan：环境检测使用 Windows 自带 PowerShell

[Spec](../spec/environment-powershell-baseline.spec.md)。直接实施，不调度 roadmap。沿用证据根 `C:/SteerlineDebug/wt-5069-local-orca-environment/`，每次独立新输出，不部署。

1. PSB-A01：同步固定 generation/execution/testing 材料与项目镜像；`.ps1` manifest 使用 powershell.exe。更新实际调用及受影响夹具，解释器由文档固定，不按机器临时回退。
2. PSB-A02：统一替换 PS7 JSON/摘要/文件 API；共享 writer 仅解析协议规定的严格 JSON 布尔对象并保留 Ordinal 键、锁、刷新和原子替换。308 只调整正式 L1/L2 与受管协议必经调用，保留完整选择与通过标准。
3. 实施中验证 PSB-A02/A03：真实 5.1 子进程运行三仓 TTY/失败报告回归、SteerlineTest L2 原报告绑定、根仓进度 writer 严格 JSON/Unicode/并发互操作、308 报告/进度协议。使用不含 pwsh 的 PATH；逐项核对非零执行、拒绝项原始字节与清理，不将夹具等同 Unity 业务验收。
4. 独立代码审核后提交冻结源码，再以干净固定输入验证 PSB-A01–A03：根仓正式 L1/L2、定向 environmentadaptation/prompts/runtimeprogress，以及 controlled L3 `testing.project-environment-readiness`；SteerlineTest 正式 L1/L2；308 通过原正式 L1/L2 执行，缺业务依赖或失败如实保留。文档链接、清单一致性、同源正文与 diff 检查。
5. PSB-A04：核实当前 Orca runtime、旧测试驱动已结束及原未知资源边界；固定干净根仓/项目提交、已有独立 Runner/profile、新输出，运行正式 live L4 `testing.project-environment-local`。原 Run 由正式入口停止并封存，不向原 worker 补发输入、不借旧结果或直接关闭未知 worker。
6. 独立复审实际正文、字节、代码、测试及本机原生证据，修正确认问题。专属验收登记每轮输入/报告/失败与未验边界；提交验收记录并更新现有 PR，不合并或部署。
