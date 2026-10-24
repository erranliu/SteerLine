# 单文件运行时测试进度

SteerLine 从冻结安排生成 `test.<test_id>: false` 对象，base64 编码后随候选公开 submit 的 `progress_file.initial_content_base64` 提交。Runner 在真实工作区绑定单文件，通过 `STEERLINE_PROGRESS_FILE` 提供路径；不理解 JSON 或测试含义。

入口开始/重跑置 false；完整通过后置 true。失败、跳过、前置失败、零匹配和不完整选择均未完成。SteerLine 查询 `progress-file --run-id R`，核验身份、版本和原始字节，再展示已通过/未完成。断连和格式错误保留最后有效快照并显示诊断，进度不改变执行终态。

```json
{"test.l1.minimum":false,"test.smoke.delivery":true}
```

## 写入入口

```powershell
./tools/report-progress.ps1 -Name test.l1.minimum -Reset
# 正式测试入口在真实完整通过后调用
./tools/report-progress.ps1 -Name test.l1.minimum -Completed
```

脚本操作整个环境绑定文件中的既有字段，不接受自选路径，不新增字段。底层 `tools/testing/RuntimeProgress.ps1` 供项目入口复用，未注入变量时无操作；自动入口只处理本次文件声明的 ID。读改写由同目录 `.lock` 文件字节 0 的 1 字节排他锁串行，临时文件完整刷新后原子替换；锁文件不删除，进程退出释放锁。

SteerLine `cmd/test-feedback` 的 L1、L2、L3 和 L4 使用原真实 summary/场景清理校验决定 true。通用 Go 与 PowerShell 写者使用相同文件和锁协议。业务项目采用相同脚本，由实际入口的选择、用例、输入和清理核验结果驱动，不由模型填报测试通过。

## 记录及部署边界

候选不再调用 R2 artifacts 或 workspace-git，不要求 managed 报告及其 summary 字节预算。原项目日志仍保存，独立 `test-feedback record` 沿主干机制将标准报告导出为 Git JSON，历史页只读已核实主分支；布尔进度不会自动生成详细历史。

新库 schema 为 31（主干 Git 历史已使用 30），原开发 schema 明确拒绝，不做迁移、双写或回退。本次源代码改造不更新安装版，不接管或补写旧在途候选。新候选须使用发布 progress-file 能力的 Runner。

Plan 自定义目标声明/解析/注入暂未实现，首期固定测试字段。后续扩展仍由 SteerLine 定义语义，Runner 通道保持不变。

[Spec](../../SteerlineFlow/spec/runtime-test-progress.spec.md)、[Plan](../../SteerlineFlow/plans/runtime-test-progress.plan.md)、[验收](../../SteerlineFlow/testing/acceptance/runtime-test-progress.md)。
