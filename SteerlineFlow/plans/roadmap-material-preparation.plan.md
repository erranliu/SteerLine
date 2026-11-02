# Roadmap 材料自动准备 Plan

依据 [Spec](../spec/roadmap-material-preparation.spec.md)。保留本聊天先前 LF/CRLF 修复及旧请求，不使用安装版调试。

## 实施

1. Runner 复用已有 Document、Material、writeFixed 和 verifyFrozenMaterials，增加 generate 的 file 材料、公开能力/schema 和结果 manifest；正文不进入 contentParts。独立克隆位于仓库外 `C:\SteerlineDebug\wt-762b-roadmap-materials\runner-source`，原 Runner checkout 保留；不使嵌套仓库干扰本项目源码守卫。
2. SteerLine 复用公开客户端/内容队列/原请求持久化，只生成 `roadmap-inputs/` 下的原文、已有图/映射及确认规划文件。静态规范仍从固定提交读取；不建立临时 Git 提交、第二套执行链路或动态材料数据库。
3. 短提示明确区分固定 Git 与本次文件材料；初始化从唯一嵌入规则来源交付静态文件及默认 SpecGuide。新增窄 CLI，更新项目准备说明和路径文档。
4. 去除受控及真实整链夹具中的手工 JSON 准备，用正式初始化入口准备静态规则；核对真实只读模型读取、文档 PR/入图及停止资源。引用摘要由平台核验原 Git/输入后填写，结果等待 completed/stopped 后采用。按实际记录更新验收和独立审核结论。

## 测试计划

环境：Windows、Go/Git/Node；静态与受控无需模型。真实验证用独立构建 Runner、本机 Codex 和独占临时项目/home/provider，默认分支及安装版不受影响。证据采用 [paths.md](../paths.md) 的外部唯一目录，每次独立叶目录；全部结果登记 [专属验收](../testing/acceptance/roadmap-material-preparation.md)。

| 覆盖 | 执行 | 最小通过条件 |
| --- | --- | --- |
| RM-A01 | CLI/roadmap 初始化 owner 单测 | 创建/幂等/只读/保留指南与冲突、重定向反例通过 |
| RM-A02–A03 | Runner contracts/resources/application 定向；SteerLine roadmap/runnerclient/app 定向 | 文件未内联、原字节/独立 manifest 正确；路径/内容/缺能力/幂等反例全部通过 |
| RM-A02–A04 | `go run ./cmd/test-feedback -- ./internal/roadmap/... ./internal/runnerclient ./internal/app -run 'TestRoadmap' -count=1`（材料及客户端反例另选实际测试名） | 受控正式路径不准备动态 JSON，完整文档/发布/入图和原请求/停止一致 |
| RM-A04 | `go run ./cmd/test-feedback -- -tags=e2e ./internal/app -run '^TestRoadmapLocalCodexPromotion$' -count=1 -timeout=20m`；固定 `STEERRUNNER_EXE`、`STEERLINE_ROADMAP_EVIDENCE` 和公开构建身份 | 真实模型读取文件并生成完整可审阅文档；实际 Git/provider/SQLite DAG、同源 manifest 与原生 stopped/清理全部通过 |
| 回归 | SteerLine 固定 L1/L2/web.unit；Runner 固定 L1、受影响 owner 与本次真实 generate 专项 | 实际记录失败及基线复现；不将专项扩大为 Runner 全量 L2/发布资格 |
| 治理 | 两仓提示词/接线断言、文档链接及 diff 检查，独立只读审核 | 正文实际差异和字节数可复核；安全边界未被样本或检查程序放宽 |

真实外部 PR、安装部署不以受控结果代替；有需要按授权追加并单独登记，不把未知发送或资源未结算视为通过。
