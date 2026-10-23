# Web 首次启动空执行列表修复验收

日期：2026-10-21。普通问题排查与修复，不属于 roadmap 执行任务。

## 原因与范围

安装构建 `64d6f321652e509826360f933ce080659f39bbeb` 的 merge-h-308-code 项目只读取证：任务 #3 为 open、依赖就绪且无启动 blocker，派发为 manual/unpaused；远端 `orca-remote-desktop64g-xhigh` 方案可用。任务接口实际返回 `workflow_runs: null`，空执行快照返回 `runs: null`；数据库没有候选或启动命令记录。

Web 首次确认启动直接对 `task.workflow_runs` 调用 map/filter，在提交前抛异常，未产生可展示回执。此前测试夹具均使用空数组，未覆盖真实空值。候选生成后，任务详情还会直接读取 `page.runs.some`，存在同类空值错误。

本次将前端类型与实际可空接口对齐，空列表按零候选处理；等待首次领取后重读仍须包含原候选身份，空列表不放行后续提交。不修改工作流、模型参数、恢复授权或安装数据。

## 自动验收

环境：工作区 Vitest/jsdom 隔离夹具，mock 提交边界；TypeScript/Vite 构建。没有启动安装项目、远端 Orca 或模型，没有真实 PR 副作用。

| ID | 验收项 | 入口与结果 |
| --- | --- | --- |
| NRS-A01 | 实际接口形状的两个 null 列表，首次确认远端 Luna xhigh 方案后仅提交一次 start_task，保持所选 profile、任务版本和目标，显示选中的正在提交卡片 | `src/features/runs/candidate-starts.test.tsx` 新增场景；修复前失败：零提交且 `Cannot read properties of null (reading 'map')`；修复后通过 |
| NRS-A02 | 原首次领取后，重读缺少原候选（含 null）时，等待中的候选保持未提交，零额外副作用 | `src/app/use-candidate-starts.test.tsx` 的 version/graph/identity/runs 反例通过 |
| NRS-A03 | 原多候选上限、顺序、迟到回执、重启与任务详情交互维持通过；可空类型通过构建检查 | 下列定向测试 19 项通过，Web build 通过 |

仓库根目录执行：

```powershell
npm --prefix web test -- src/features/runs/candidate-starts.test.tsx src/app/use-candidate-starts.test.tsx src/features/tasks/task-detail.test.tsx
npm --prefix web run build
```

运行结果保留在本次聊天工具输出；场景与断言随本文件同提交维护。未运行全量、L2/L3/L4：本次为前端空值处理，定向组件/提交边界回归覆盖故障，不需要远端执行现场。

## 待验

- 安装环境未修改、未重启、未部署；安装版仍包含原问题。
- 更新后由用户实际启动 merge-h-308-code 项目并检查远端运行，尚未验收；本次提交成功的夹具证明不代表远端 Orca 或模型执行通过。
