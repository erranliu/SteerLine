# 任务执行页视觉调整验收

日期 2026-11-12。改版前使用提交 `0b0e1bf5` 的生产构建，改版后使用同一提交加当前工作树改动的生产构建。验证使用隔离 Edge 和模拟 API，未部署到安装版。

- 运行中可直接看到当前阶段、状态与快照观察时间。最近消息使用较大的正文和较宽的行距，阶段变化按时间线显示。
- 待回答保留原问题、上下文、选项和草稿编辑。桌面截图中「回复并继续」完整可见。对照用例验证选中选项后提交按钮可用，另由 B5 验证编辑与提交。
- 对照图使用相同项目、候选、模拟时间和 1440×1050 视口。新版待回答用例另外在 800、375、320 像素宽度验证编辑框可见、页面无横向溢出。

## 验证与证据

测试证据位于 `C:/SteerlineDebug/wt-82d4-runs-visual/`。

| 命令或场景 | 结果及证据 |
| --- | --- |
| `npm --prefix web run build` | 前端生产构建通过。 |
| `npm --prefix web test -- src/features/runs` | 20 个文件、129 项通过。移除待回答区重复徽标后，定向复跑 `answer.test.tsx`，7 项通过。 |
| Playwright 改版前对照 | 1 项通过，见 `diagnostics/browser-before-69036e7dccc345608e8ade9d4f1b66ff`。 |
| Playwright 新版对照及 `ui-simplification.spec.ts` | 6 项通过，见 `diagnostics/browser-after-570dcccb14e14394a9b1c3333c77db9b`。 |
| Playwright 最终版对照、B2、B5 | 3 项通过，见 `diagnostics/browser-final-0f3480f75e1e40d890df89e8eadc1ead`。 |
| `go run ./cmd/test-feedback -level=L1` | 通过，见 `test-results/L1/20261112T103119040Z-644571d539974d20afbc1c3097ccb3d8/summary.json`。 |
| `go run ./cmd/test-feedback -level=L2` | 受控交付冒烟通过，场景资源已清理，见 `test-results/L2/20261112T103119040Z-644571d539974d20afbc1c3097ccb3d8/summary.json`。 |

原尺寸截图交付为 `before-running.png`、`after-running.png`、`before-waiting.png`、`after-waiting.png`。
