# P01 身份断链手动结算证据

时间：2026-11-09 15:40 Asia/Shanghai。结论：P01 planning complete；仅指 PLAN 交付完成，不表示 Worker 实现、运行测试、E2E 或真实业务 API 验证完成。

## 交付核验

- PLAN：`/Users/echo/orca/workspaces/obot/openapi-shared-worker/docs/plans/openapi-workers/PLAN.md`，30070 bytes。
- 00 核验该 worktree 的未跟踪文件仅此 PLAN；无业务实现、依赖锁、生成产物或全局配置修改。
- 00 核验章节齐全，并确认 PLAN 覆盖 Obot 本地源码、官方 `mcp-images` tag `v0.27.2`、FastMCP 3.4.7 pinned wheel、快照/摘要、operation 映射、编译隔离、缓存、HTTP 执行、取消、测试、失败行为和交付物。
- 00 核验无行尾空白。PLAN 将源码事实、上游源码事实、候选设计和未验证边界分开。

## 必须进入交叉审核的发现

- 官方 v0.27.2 `run_http_async(... stateless_http=True)` 未设置 `json_response=True`；草案的无 SSE 要求与现有 `/mcp` 行为存在直接冲突。共享 Worker 计划是不暴露 `/mcp`，由 02 使用 `describe/execute` 适配。
- FastMCP `OpenAPITool.run()` 完整缓冲并 JSON 化响应，把业务错误变成工具异常，不能作为共用执行核心；P01 建议保留其 OpenAPI IR 与工具名兼容性，另实现有界原始 HTTP 执行器。
- `snapshot_required` 只能证明缓存未命中阶段未触达上游；补送后的传输失败、崩溃或超时不能证明上游未执行，必须禁止自动重试，不能宣称通用 exactly-once。
- public-only egress 策略可能与公司内网 API 冲突；允许私网必须是显式部署策略并继续做 DNS 全答案校验与 origin 固定。

## 身份断链处理

- 新 Codex 终端完成 PLAN 后，`worker_done` 被 Orca 拒绝：`worker_identity_changed`，当前进程不再是 Dispatch `ctx_69a8fa771177` 的 exact owner。
- `worker-read` 证明旧 structured worker terminal 已 exited。
- 00 对旧 Dispatch 执行 `worker-abandon`：仅 fence assignment、无进程或文件系统动作，保留 possibly-live resources。
- 00 随后将 Task `task_cb50109a42a3` 手动设为 completed；这不改写 Dispatch 的 abandoned 历史。

## 后续

- P01/P02 已完成，P03 Dispatch `ctx_d6df8fe45947` 与 P04 Dispatch `ctx_29e62fe13483` 继续保持最多两个规划 worker。
- 全部五份 PLAN 齐备前，交叉审核和 G00 冻结仍禁止启动；I01 仍等待 G00。
