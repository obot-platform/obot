# 初次编排启动证据

日期：2026-11-09。结论：verified，仅限下列编排事实。

## Run 与依赖

Run：`run_e4b29a6d58f8`。

00 coordinator mailbox：`orca_session_id:codex_5e094850_e036_4c5b_954b_770e6ba7ff51`。

Orca task-list --brief 已确认五个规划任务 ready；五个审核任务依赖所有规划任务；G00 依赖所有审核任务；五个实施任务均依赖 G00；I03 额外依赖 I01/I02，I05 额外依赖 I01—I04。完整 ID 见 ORCA-INDEX。

## P01 启动回执

```json
{
  "ok": true,
  "result": {
    "dispatchId": "ctx_69a8fa771177",
    "taskId": "task_cb50109a42a3",
    "runId": "run_e4b29a6d58f8",
    "state": "ready",
    "agent": "codex",
    "mode": "structured",
    "terminal": "structworker_487a01e9-06db-4487-af44-8f371c9ed4c3",
    "workspace": "c7918928-0734-4283-b8b2-8bc85657f341::/Users/echo/orca/workspaces/obot/openapi-shared-worker"
  }
}
```

## P02 启动回执

```json
{
  "ok": true,
  "result": {
    "dispatchId": "ctx_437a1bece6ea",
    "taskId": "task_e3068b23e0ec",
    "runId": "run_e4b29a6d58f8",
    "state": "ready",
    "agent": "codex",
    "mode": "structured",
    "terminal": "structworker_4b48b5b6-e82b-4160-8551-d2c72bd85534",
    "workspace": "c7918928-0734-4283-b8b2-8bc85657f341::/Users/echo/orca/workspaces/obot/openapi-worker-bridge"
  }
}
```

启动使用真实既有 Task、完整既有 worktree selector、agent=codex、显式 Run 和 00 coordinator；未传入新 worktree 创建参数或指定用户未要求的模型。

## 文件与保护检查

- 00 的 PLAN、ROADMAP、CONTRACTS、ACCEPTANCE、ORCA-INDEX 均已存在且非空；五份规划 brief 的公共说明路径为绝对路径；必要空白检查无问题。
- 00 git status 仅显示新增 docs/plans；没有修改业务实现。
- 原工作区 git status 仍为 skin/ztocc-theme，README.md 为用户原有未提交修改。

## 不代表的结论

以上没有证明五份 feature PLAN 已完成、交叉审核通过、契约冻结、实现完成、E2E 通过或真实集群部署。当前首批两个 Codex 执行的是规划任务；00 等待 Orca 原生回报唤醒，不启动自旋监控脚本。
