# 环境检测复用现有 checkout 验收

2026-11-13，SteerLine 在现有执行配置上发送 Runner checkout_probe 结构化请求。材料从固定基线读取，项目根、入口和仓库外结果由 Runner 执行和核实。

## 提示词变更和独立审核

删除 environment-invoke.md 全文，规范 UTF-8 源文件 412 → 0 字节。代表 PowerShell 请求正文从 396 → 0 字节，完整删除差异见该 golden 文件和原模板的 Git diff。删除原组装函数、发送接线和对应源码守卫例外。命令参数和材料摘要由结构化请求传入，模型不参与项目检测。独立审核核对了实际请求与源码守卫，并独立运行提示词和边界测试通过。

## 合并前验证

- L1 通过。sl-environment-checkout-final-l1-dfc3fd110cd24515b5953d5002a9fab9/summary.json。
- L2 通过。sl-environment-checkout-final-l2-a86366df55b244569cf244983d719597/summary.json。
- 应用公开环境命令、检测材料和依赖定向验证通过。sl-checkout-final-app-eda6f7d2b5154bb7a13b0d5e4106aaf6/summary.json。
- Markdown 链接与 git diff --check 通过。
- 配套 Runner 用独立服务和临时 Git 项目验证了本机和本机 Orca 结果封存，配对机验证见 SteerRunner PR 161。没有更换正式安装。

配对机 308 验证创建 Run 56c1554838e7b2d7563f3fda32bb0fb5。其 checkout 的 HEAD 3dd85978 与调用基线 d57552a2 的 manifest 依赖声明不同，Runner 在调用项目入口前按约定拒绝，原 Run 已停止并封存诊断。未修改目标 checkout。
