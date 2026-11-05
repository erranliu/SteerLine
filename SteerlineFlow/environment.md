# SteerLine 项目环境要求

所有运行机和执行链路共用本文与检测脚本；资格按项目 × 运行机 × 链路分别检测和保存。[接口指导](../SteerlineProjectGuideDoc/environment-adaptation.md) · [共用材料规格](spec/shared-environment-materials.spec.md)。

```steerline-environment
{
  "version": 5,
  "script": "SteerlineFlow/scripts/verify-environment.ps1",
  "interpreter": "powershell.exe",
  "dependencies": ["go.mod"]
}
```

## 要求与准备

本项目当前声明 Windows PowerShell 5.1 入口。目标须提供 PATH 可定位的 Git、Go（不低于 go.mod 的 go 版本）、Node 24+、npm 和 Python 3。请通过各工具正常安装方式准备，重新打开执行环境后再发起检测。脚本只执行版本查询，每项最多等待 15 秒；不安装工具、不修改 PATH、不访问安装现场。

检测仅确认以上项目基础工具可用，不运行项目测试、构建验收或 PR 发布，不检测 Codex/Orca、TTY、Runner、执行链路或模型。浏览器及真实外部集成所需的专项条件由[测试流程](testing.md)分别处理；基础工具就绪不表示这些专项或项目测试通过。

## 调用与诊断

SteerLine/Runner 负责目标连接、启动、结果通道、停止和封存；链路不能执行时显示调用失败。脚本在固定源码工作区根运行，从 `STEERLINE_RESULT_FILE` 读取预置身份，保留关联字段并原子写回 report3 的 completed、ready、message 和工具检查明细。工具缺失、版本不足或查询失败时返回可读 not_ready；脚本正常完成时退出 0，不能读取/写回结果时异常退出。

脚本按相同标准检测当前机器，不根据环境键、adapter 或目标名称改变规则。报告身份仍绑定本次目标，不能跨链路借用。公共文档、入口或 go.mod 改变时，各已获得资格的目标分别沿原规则失效并重新检测；不自动执行检测。项目测试、业务文档、Spec/Plan、roadmap 不进入环境资格依据。
