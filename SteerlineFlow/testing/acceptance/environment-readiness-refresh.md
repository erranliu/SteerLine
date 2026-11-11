# 环境检测结算后的就绪状态刷新

## 现场与改动

2026-11-11，`merge-h-308-code` 的检测记录 `9892c442768b4d528791b03440fe7e40`、Runner Run `623ff4a66523333f147316f48bdfde9f` 和 Attempt `c63df5842c266cd44a73605feb0b52ee` 在安装现场的 SteerLine 记录中匹配。按北京时间，Runner 在 14:42:11.939 更新到已停止，14:42:12.069 封存结果。SteerLine 最后一次观察为 14:42:13.623，记录为 `ready`。现场缺少浏览器采样，无法确定该次页面实际切换时刻。

检测详情每 3 秒轮询，当前资格原先每 15 秒轮询，两者可能在不同时间显示结算结果。在线且页面可见时，只要当前环境列表中有未结算检测，资格查询也改为每 3 秒轮询，全部结算后恢复 15 秒。就绪仍使用服务端资格查询的结果。

## 验证

受测源码为 `99aa0cc9f94d803716c9b753f274e6ba0cb01c5a` 加工作树改动。`npm.cmd --prefix web test -- src/features/environment-checks src/features/tasks/dispatch.test.tsx` 通过 37 项，覆盖自动更新当前状态与结算后恢复原轮询间隔。`web/node_modules/.bin/tsc.cmd -b web` 通过。

固定测试以 `go run ./cmd/test-feedback -level=L1 -out <输出目录>` 和 `go run ./cmd/test-feedback -level=L2 -out <输出目录>` 执行，均通过。沙箱内运行曾因原子替换文件和初始化临时 Git 仓库的权限失败，最终在沙箱外使用独立测试现场通过。报告位于 `C:\Users\lmas1\.codex\visualizations\2026\11\11\01a1c964-ded6-7d42-ad2c-a280b1b04565` 下的 `L1-98031e6097f34233b6f0e7af0c4c8ca9/summary.json` 和 `L2-98031e6097f34233b6f0e7af0c4c8ca9/summary.json`。

未部署，未重新执行真实环境检测。
