# Orca 重导入恢复接管证据

时间：2026-11-09 15:33 Asia/Shanghai。结论：verified，仅限本文件列出的恢复事实；五份 feature PLAN 与后续阶段仍未完成。

## 恢复检查

- 当前 Orca repo：`8acef906-8a50-4889-8580-9f57625d0a0d`；历史 repo：`c7918928-0734-4283-b8b2-8bc85657f341`。未重建分支、worktree 或 Run。
- 新 00 会话已通过 `run-use` 绑定既有 Run `run_e4b29a6d58f8`。`task-list --full --json` 确认 16 个 Task 与原依赖完整：P01/P02 dispatched，P03/P04/P05 ready，R/G/I 按 DAG pending。
- 新 00 coordinator mailbox：`orca_session_id:codex_6a58d271_1826_49d6_8402_3211beced2c1`。旧 `codex_5e094850_e036_4c5b_954b_770e6ba7ff51` 仅作为历史记录，不再作为回报入口。
- `git worktree list` 与逐目录 `git status` 确认：原工作区 `/Users/echo/mcp-platforms/obot` 仍只有用户已有 `README.md` 修改；00 只有未提交 `docs/plans/`；01—05 分支和 HEAD 均为 `feature/openapi-*` @ `2be04f025d1c86144e8670d9e9341b263edc28ea`，除 00 外均干净。
- 15 个既有计划/索引/证据文件完整；01—05 恢复检查时均未落盘 PLAN。

## P01/P02 映射

| Feature | Task | 原 Dispatch | 当前 live Codex terminal | 恢复消息 |
| --- | --- | --- | --- | --- |
| P01 共享 Worker 规划 | task_cb50109a42a3 | ctx_69a8fa771177 | term_ef1559c4-153a-43de-a9b9-5b62881d039f | msg_379ecd0ccca1 |
| P02 Obot 接入规划 | task_e3068b23e0ec | ctx_437a1bece6ea | term_570a4f6d-a834-405e-a622-bc24b4a5ed03 | msg_b373bf0e0451 |

恢复消息要求二者继续原规划、只写各自 `docs/plans/openapi-workers/PLAN.md`，并使用原 Task/Dispatch 执行 `worker_done`。不得把 live terminal 证明为业务实施、测试通过或任务完成。

## 防重复处理

- 一次性 automation `2b76e693-617c-43bd-bc4d-1b10bf8a7243` 已由新 00 禁用；其旧 repo/workspace ID 保留为历史，不再允许触发重复唤醒。
- 当前活跃规划 worker 数为 2，因此不启动 P03/P04/P05，也不派发任何 I 任务。
- 未执行 commit/push、生产部署、真实 JWT/业务 API 调用、集群操作或全局 Codex 配置修改。
