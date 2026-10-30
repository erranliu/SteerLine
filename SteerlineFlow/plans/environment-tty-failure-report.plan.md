# Plan：TTY前置失败的项目报告收尾

[Spec](../spec/environment-tty-failure-report.spec.md)。直接实施，不调度roadmap、不部署。证据根 `C:/SteerlineDebug/wt-5069-tty-failure-report/`，每次新目录。

1. 独立审核最小边界；在现有environment-tty.ps1补负报告转换函数，由各项目verify-environment.ps1的tty-failure分支复用原原子写入，不建新报告框架。SteerLine、SteerlineTest、308同步；保持原业务检查和测试标准。
2. 固定execution/generation规则和镜像明确失败后只记录、不换封装继续检测；项目文档给出命令，正常入口不放宽。保存实际diff、UTF-8字节/SHA和业务原因，独立批准后更新摘要/黄金样本。
3. TFR-A01：扩展各项目现有scripts/test-environment-tty.py，以真实pwsh子进程和隔离伪工具验证固定failed、完整ID、保留同轮事实/原身份、无外部命令、非法输入不改两文件；修正该测试此前遗留report1夹具为当前report2。入口在任何Git/Check/L2调用前分流；超时/未知生产者不伪造结算。SteerlineTest另跑原完整L1/L2；308本轮不启动Unity或修改配置，记录其静态/报告入口验证范围。
4. TFR-A02/A03：test-feedback定向执行./internal/environmentadaptation/...与./internal/prompts，新增三轮TTY负报告正常终结反例；固定干净提交跑L1、完整L2及受影响readiness L3。缺/失败TTY仍禁止ready。
5. TFR-A04：固定同源308新提交、原远端profile/独立Runner/1200秒预算，运行正式testing.project-environment-orca，新out；允许真实环境负结果，验收只在原报告完成、身份/停止封存、预算终结及清理均核实后声明失败报告闭环通过，不将正式正向L4失败改写为通过。若原工具完全不可用如实未验，禁止调用方代写result或伪造故障。
6. 独立复核实现/正文及原生证据，链接/清单一致性/diff检查，提交项目材料与SteerLine可审核PR，不自动合并或部署。
