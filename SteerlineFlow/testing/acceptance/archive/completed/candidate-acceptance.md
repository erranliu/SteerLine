# #39 本机多候选验收摘要

本机 Codex 与本机 Orca 的 CN1–CN8 自动验收已完成；人工待验为空，#38 CC5 另见[用户确认](manual-acceptance.md)。远端不在本次范围，也不证明 #42 的固定安装构建、24 小时或完整组合验收。

最终组合固定源码 `919a4ce755eb6d56759efa479608b75bef34749c`，main `73e5692258414166c37bea1663f45fce8008d219`：定向 41 项、L1 七组（278.2 秒）、`candidates.local`（1153.3 秒）通过，无 fail/skip。36 个完整 Body 页校验和通过，逐候选 usage 为正，清理完成。胜出测试 PR #77 与后继 #79 真实合并，中心唯一 Delivery；原反馈重启、故障兄弟继续、放弃/新增、合并后拒绝及落选在途更新均核对原身份。

原 CN1–CN8 构建 `6c3e1735b7b71adba834f4fdbb2145a9881d8e12` 的 local-08 场景也已通过；此前失败、超时与 pending PR 修复保留为历史事实，不由最终通过覆盖。精确断言、测试 PR、身份、版本和清理见[原验收报告](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/candidate-native-acceptance/README.md)、[验收索引](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/candidate-native-acceptance/acceptance.json)及[协调结果](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/candidate-native-acceptance/coordinator-final.json)。

当前重新执行方法及 manifest 维护在[E2E 指南](../../../../../tests/e2e/README.md#本机候选-manifest)，每次使用实际 HEAD 和新的空输出目录，不直接重用旧 manifest。
