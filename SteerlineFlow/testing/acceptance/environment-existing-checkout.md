# 环境检测复用现有 checkout 验收

2026-11-13，SteerLine 在现有执行配置上发送 Runner checkout_probe 结构化请求。材料从固定基线读取，项目根、入口和仓库外结果由 Runner 执行和核实。

## 提示词变更和独立审核

删除 environment-invoke.md 全文，规范 UTF-8 源文件 412 → 0 字节。代表 PowerShell 请求正文从 396 → 0 字节，完整删除差异见该 golden 文件和原模板的 Git diff。删除原组装函数、发送接线和对应源码守卫例外。命令参数和材料摘要由结构化请求传入，模型不参与项目检测。独立审核核对了实际请求与源码守卫，并独立运行提示词和边界测试通过。

合入 `main` 的 `21f78813238c0f792099044be39da8fbf67ae62d` 后，基线包含 [PR #562 的等待退出补丁](https://github.com/erranliu/SteerLine/pull/562/files)。相对该基线，[短提示模板](https://github.com/erranliu/SteerLine/blob/21f78813238c0f792099044be39da8fbf67ae62d/internal/prompts/environment-invoke.md)和[代表 PowerShell 请求正文](https://github.com/erranliu/SteerLine/blob/21f78813238c0f792099044be39da8fbf67ae62d/internal/prompts/testdata/environment-invoke.golden.md)均为整文件删除，UTF-8 分别为 704 → 0 和 688 → 0 字节。独立审核确认请求只传调用参数和材料摘要，Runner 等待原脚本实际退出并核实退出结果后封存。合并保留主干的材料缓存和独立结果文件类型。

## 合并前验证

- L1 通过。sl-environment-checkout-final-l1-dfc3fd110cd24515b5953d5002a9fab9/summary.json。
- L2 通过。sl-environment-checkout-final-l2-a86366df55b244569cf244983d719597/summary.json。
- 应用公开环境命令、检测材料和依赖定向验证通过。sl-checkout-final-app-eda6f7d2b5154bb7a13b0d5e4106aaf6/summary.json。
- Markdown 链接与 git diff --check 通过。
- 配套 Runner 用独立服务和临时 Git 项目验证了本机和本机 Orca 结果封存，配对机验证见 SteerRunner PR 161。没有更换正式安装。

配对机 308 验证创建 Run 56c1554838e7b2d7563f3fda32bb0fb5。其 checkout 的 HEAD 3dd85978 与调用基线 d57552a2 的 manifest 依赖声明不同，Runner 在调用项目入口前按约定拒绝，原 Run 已停止并封存诊断。未修改目标 checkout。

## 合入 main 后的验证

- 合并工作树运行 `go run ./cmd/test-feedback -level=L1` 通过，报告 `%LOCALAPPDATA%/Temp/steerline-pr546-l1-8935b6a84abc4e8283eb780c53a137fa/summary.json`。合并提交 `8f6d13addc32d9ca0ec46c525ee0a9202ad994fa` 的 [GitHub 提示词治理检查](https://github.com/erranliu/SteerLine/actions/runs/43794795657/job/133061041496)通过。
- 合并提交运行 `go run ./cmd/test-feedback -level=L2` 和 `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.environment` 通过。报告分别为 `%LOCALAPPDATA%/Temp/steerline-pr546-l2-a0d2758445884baeb69c9afc8fda4f09/summary.json` 和 `%LOCALAPPDATA%/Temp/steerline-pr546-environment-1b6b2f5b2f584691a9b7e5c6db2fb207/summary.json`。环境专项包含缓存命中后校验 manifest、入口和依赖摘要的回归断言。
- 首次在 Windows 沙箱内运行 L2 和环境专项遇到进程或文件权限错误，以上结果来自沙箱外的独立临时目录。本轮未在真实配对机执行快进。
