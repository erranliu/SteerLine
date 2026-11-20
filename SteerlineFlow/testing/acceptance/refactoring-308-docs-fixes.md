# 308 文档维护阻碍修复与真实交付

2026-11-19，承接[修复前的真实测试](refactoring-308-docs-live.md)，用户要求修复所发现的问题。当前聊天直接修改 SteerLine 和独立 Runner worktree，未部署，未操作原安装任务。最终在真实 `merge-h-308-code` 的同一 `AIRefactor` 基线完成文档维护交付。

## 修复

Runner 创建 worktree 的临时 `core.hooksPath` 改用 `os.DevNull`。Windows 对应 `NUL`，Unix 仍为 `/dev/null`，不持久修改项目 Git 配置。真实 Git LFS 不再把 Unix 路径当作工作区内的 `dev/null` 并安装四个 hook。保留 LFS 内容过滤、原项目 hooks 和工作区干净门禁，没有清除实际工作区文件以制造通过。

SteerLine 的维护决策 Schema 允许可选 `question` 为 object 或 null；消费时将 null 归一为缺省。`ask` 仍拒绝缺少有效问题，非 `ask` 仍拒绝携带问题对象，阶段角色权限不变。不增加旧格式读取、迁移、协议回退或结果兜底。

## 实际正文差异与独立审查

固定 `internal/prompts/maintenance-decision.md`、组装接线和黄金样本未修改。实际发送的 Schema 只有以下正文差异：

```diff
- "question":{"type":"object", ...}
+ "question":{"type":["object","null"], ...}
```

UTF-8 字节数：first/select 的 Schema **795 → 804**，audit 的 Schema **804 → 813**；固定决策模板 **3135 → 3135**。增长来自可选字段的明确空值类型，不增加自然语言规则。业务依据是修复前真实模型已返回正确的 execute 决策但附带 `question:null`，被 Schema 拦截。

按 AGENTS.md 的独立审查要求，`/root/review_308_fixes` 只读审核两处生产补丁、实际 Schema 字节差异及回归断言；测试归属调整后再次复核，结论通过，无确认缺陷。审核确认没有放宽人工提问、干净门禁或阶段权限，也没有修改边界守卫及白名单。独立审查不代表远端分支保护或合并审批。

## 验证

证据根：`C:/SteerlineDebug/wt-9ed5-308-docs/`。

- Runner 完整 workspace owner 包通过：`diagnostics/runner-workspace-fixed-01/tests.jsonl`、`result.json`。含新增 Windows 真实 Git LFS 回归：微型本地对象确实物化、HEAD 固定、工作区干净、没有 `dev/`，原 checkout hook 未执行且未改写。需要已安装 Git LFS，不联网、不调用模型；测试指南已声明该前置。首轮沙箱运行受 junction/真实 git-lfs clean 的系统权限限制，未作为通过；随后在正常本机权限下保持原断言完成整包。
- SteerLine maintenance、refactoring、refactoring/model 和 prompts 定向 owner 测试通过。最终 null/角色/Schema 互操作 JSON 证据为 `diagnostics/schema-fixed-01/tests.jsonl`、`result.json`，覆盖各阶段及有效 ask、无问题/null ask、非 ask 的完整问题对象等拒绝边界。
- 首轮 L1 `test-results/L1/schema-fix-01/summary.json` 失败：新增测试把第三方 Schema 编译器引入纯 maintenance 包，原三层边界守卫准确拦截。保持守卫及白名单不变，将 Schema 互操作测试移到 `internal/testkit/boundary/maintenance_schema_test.go`；纯包仅保留业务角色测试。完整边界包复验通过，独立审核复核测试归属调整未弱化断言。
- 最终正式 L1 `test-results/L1/schema-fix-02/summary.json` 通过，准备 0.49 秒、执行 21.74 秒。
- 最终正式 L2 `test-results/L2/schema-fix-01/summary.json` 通过，准备 10.56 秒、执行 94.58 秒，清理完成。随后才启动真实 308 流程。
- 本次不重新宣称 Runner 完整 L1/Controlled 通过；其[前次失败及资格边界](refactoring-runner-blocked.md)和 Runner 原验收记录保持。此次新增差异由完整 workspace owner、独立审查和下述真实项目链验证。

## 真实 308 补测

使用全新的 `packages/run-04`、完整对象克隆、隔离认证副本和原项目 `command_timeout=2m`；未修改项目规则或测试定义。原基线仍为 `9a084fb61a3bfdac79cf30f0ff8b3a1cc280474a`，唯一目标仍为 `docs/Code/persistence-ownership.md` 的 CloudLogin 过期说明。

- 行动 `7bfdf1bd1787450081eb5693421ec23c` 完成正式环境检测、真实选题、实施、独立验收、发布、PR 核验及独立最终评审，终态 `delivered`，无诊断。
- 原始选题再次实际返回 `question:null`，这次 Run `71147e7f2b8d5b39a36ba0d50b3516be` 完成，原文保存在 `evidence/first-run-inspect.json`，没有改写模型原结果。
- 实施提交 `13394163a6d5e5d7935d07a4d1d0fc62099a39cb`，仅改一份现行 Markdown。Runner 证据及独立 Git 核对均确认工作区干净，没有 `dev/`，`git diff --check` 退出 0。改动区分恢复时重建的 Session 服务与保留的 App 级身份/云服务/凭据和同步适配器，不改代码或资源。
- 真实 [draft PR #463](http://192.168.0.66:3000/zymobile/merge-h-308-code/pulls/463) 为 `WIP: 澄清存档备份与恢复生命周期`，head `codex/runner-1c433024882e1c41d3a7f8f70b5e360b`，base `AIRefactor`。独立 Gitea API 核对只有目标文档一项改动，PR 创建晚于接受后的发布阶段，head/base 与评审绑定一致，最终评审前后 PR 正文一致。
- 最终评审建议 `insufficient_evidence`：其材料只有 context/diff，未包含源码核验和路径检查依据，因此未建议合入。独立评审确实完成；本次“流程通过”不表示推荐合入，也不将静态文档核对称为 Unity 或业务测试通过。

原始证据为 `packages/run-04/evidence/test-result.json`、`final-view.json`、`runner-<RunID>.json`、`workspace-clean-proof.json`、`pr-verified.json`、`document-change.patch` 和 `persistence-ownership.updated.md`。这是人工 live 专项，不冒充已登记的小型 GitHub 夹具 L4 场景。

## 固定产物和清理

- Runner EXE：`artifacts/steerrunner-windows-lfs.exe`，基线 `8ebbc717776eb2ad85371060566776c4402773fa` 加本轮及此前启动修复，生产源码摘要 `75824ff79737b01e5fd974f4ea91dcc098a4095f059f6a8f6d757d0ed53de9d3`，SHA256 `31153b1f865364fc1c6a0ddef84e2596f2d67222a3ac25c904af795f853dd10f`。
- SteerLine EXE：`packages/run-04/bin/steerline.exe`，基线 `3a1777a6bbf5d4b245be2ca12aa24c4b2af4e905` 加此前 blocked 展示及本次 Schema 修复，SHA256 `7a70573365aa416e15ddfd4041c7bbbfd0043a09ab61135996080646988a1316`。
- 本轮正式 `stop`、`verify-stop` 通过，六个 Run 的资源全部结算；认证副本已删除。PR #463 保留 open draft 供用户审阅，未合并。Codex 附件工具不接受该内网 Gitea URL，使用原 PR 链接访问。
- 原 308 checkout 仍干净，HEAD 不变；安装 Runner SHA256 仍为 `0d733a1fcd09f06ad7e2891ad90030f16d3fb4fae1b877adc86fbbe1bf74137e`。未部署，未停止、恢复或重新提交原安装任务。

文档链接和 `git diff --check` 按最终记录核对；不因新增验收文本重复业务测试。
