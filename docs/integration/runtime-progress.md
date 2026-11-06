# 单文件运行时测试进度

SteerLine 定义固定测试字段与通过含义，Runner 只绑定、采集和公开文件字节，不解释测试结果。通道用于展示本候选进度，不决定执行终态、任务交付或详细测试历史；实现入口为[进度服务](../../internal/testmanagement/progress.go)。

## 字段与固定来源

候选从已就绪的冻结安排生成 `test.<test_id>: false`，经 `progress_file.initial_content_base64` 随原 submit 保存。Runner 绑定真实工作区内唯一文件，通过 `STEERLINE_PROGRESS_FILE` 提供路径；不接受模型自选位置。

```json
{"test.l1.minimum":false,"test.smoke.delivery":true}
```

完整安排保存在原候选；实际执行从同版本文件读取安排、定义和 note，按固定顺序核对真实入口，缺文件、版本或定义不符时阻断，不重新选测试。运行期协议唯一正文为[项目进度协议](../../SteerlineFlow/testing/runtime-progress.md)，正式 writer 为同版本 [RuntimeProgress.ps1](../../tools/testing/RuntimeProgress.ps1)，来源定位见[固定文件模型](../../internal/testmanagement/model/documents.go)。新请求通过固定文件供给，不在提示词内附写入器源码或 inline 兜底；原请求恢复不重渲染或补发。

## 写入入口

```powershell
./tools/report-progress.ps1 -Name test.l1.minimum -Reset
# 由真实测试入口在完整通过后调用
./tools/report-progress.ps1 -Name test.l1.minimum -Completed
```

正式 PowerShell writer 支持 Windows PowerShell 5.1，供登记入口复用；未注入变量时无操作。入口开始或重跑先清零，只有实际选择非零、无失败/意外跳过且输入、原报告与清理核验全部通过时才置 true。失败、未完成或接线失败保持 false，不用 Agent 自述、单个 exit 0 或旧报告事后填通过。

逐 ID 核对入口实际接线，文件存在或一个 dot-source 不证明全部入口已覆盖。只写本次声明的既有布尔字段，不新增 publication、PR 或报告元数据；发布及只读报告步骤不写进度。需要补接线时保持原测试选择、定义与通过要求，之后实际运行原入口。

并行写入用同目录 `.lock` 文件的字节锁串行读改写，完整刷新临时文件后原子替换；不覆盖其他字段，不删除锁文件，进程退出释放锁。其他语言写者遵守同一协议，原日志和标准报告仍由各入口保存。

## 观察与历史

SteerLine 从公开 `progress-file` 核验原 Run/input 关联、版本、字节及摘要，解析结果必须与冻结安排字段集合一致且全为 true/false。版本倒退、同版本内容变化或身份错误拒绝；断连、坏格式和采集不可用保留最后有效结果并显示诊断。进度文件版本不代替候选或 Runner revision。

布尔进度不自动生成详细报告或 Git 测试历史；独立标准报告按[报告约定](../../SteerlineFlow/testing/reports.md)保存和登记。旧 R2 artifacts/workspace-git 候选收集已移除，不重新引入其读取或门禁。持久格式归[数据模型](../product-design/data-model.md#当前格式来源)，Plan 自定义进度目标仍未实现，不因本文产生新字段。

初次实现、旧 schema 与 0/6 修复的原版本证据保留在[历史入口](../../SteerlineFlow/archive/design/runner-reliability-history.md#第四批整理来源)。本文整理只修正说明，不改协议正文、writer、提示词、黄金样本或生产接线。
