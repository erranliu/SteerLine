# 2026-10-07 验证基线

最近完整重跑见 [#238 后全项目回归与修复](all-tests-after-238-2026-10-07.md)及[结果索引](all-tests-after-238-2026-10-07.json)。全部最终 Go/E2E 报告来自 clean `6e4f1504627c4e5febf52a947d2b10e96b91773a`，rebase 基线为主线 `4eb722f16ceaccdcf3c19828213727e66bee403f`（#238）。

Full：1228 条记录、0 失败、15 默认 skip；L1：428 条、0 失败；E2E 注册表 10/10，清理和真实 usage 核验完成；带 tag 基础设施：173 条、0 失败、1 默认 sandbox preflight skip；离线 Python：28/28。采用当前固定 Windows 原生权限方案，不启用管理员 sandbox 或批量原生 opt-in。具体范围、修复、失败保留和共享任务 49 登记检查限制见本次记录。

此前基线保留于[原结果索引](baseline-results.json)及[#228 原记录](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/all-tests-after-228-2026-10-07/README.md)，只证明其原源码和环境。
