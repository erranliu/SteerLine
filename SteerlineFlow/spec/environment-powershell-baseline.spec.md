# Spec：环境检测使用 Windows 自带 PowerShell

用户确认环境检测移除 PowerShell 7 要求，并实测验收。[Plan](../plans/environment-powershell-baseline.plan.md)。沿用[环境发布边界](environment-publication-scope.spec.md)与[worker 生命周期](../../docs/product-design/worker-lifecycle.md)，不修改 Runner 或部署安装环境。

- PSB-A01：Windows 项目材料明确使用 `powershell.exe`，全部检测阶段及正式 L1/L2 必经脚本使用 Windows PowerShell 5.1 可用的语法与 .NET 能力。生成材料、解释器校验及外部取证同步；Bash 项目仍显式声明 bash，不增加解释器搜索、自动切换、旧材料读取或特殊失败记录回退。
- PSB-A02：SteerLine、SteerlineTest、308 的检测脚本和必经共享进度写入器移除 PowerShell 7 隐含依赖。UTF-8、严格布尔对象、Ordinal 唯一键、并发锁、合并与原子替换语义保留；非法输入不改原文件。单一实现，不创建另一套协议或写入器。
- PSB-A03：真实 TTY、原完整 L1/L2、真实 PR 身份、原 Run 停止、result 封存及派发门禁标准不变。受控测试验证完整脚本与正反例，不能以只执行失败记录或静态语法检查替代正向能力。
- PSB-A04：以新固定版本完成本机 Orca 原生验收，保留源码、配置、独立 Runner、原报告与资源结算证据。正确 not_ready 与环境检测流程失败分别记录；历史通过、未知 Run 和本轮结果不互相覆盖。

范围限于环境检测及其正式测试/取证调用闭包。308 非必经 Bundle 发布等工具不做全仓 PowerShell 改造。材料正文、字节变化、验证和独立审核须可追溯。
