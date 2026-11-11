# pstack 形式化技能安装证据

时间：2026-11-09 16:09 Asia/Shanghai。结论：verified。

## 安装结果

从 `backnotprop/pstack` 安装三个技能到用户级 `~/.agents/skills/`：

| 技能 | 字节数 | 用途 |
| --- | ---: | --- |
| setup-pstack | 7377 | pstack 配套配置说明 |
| create-formal-verification-skill | 5626 | 选择高代价性质、生成可复跑项目本地验证 skill |
| statechart | 9100 | 运行 TLA+ 模型检查并生成 Mermaid 状态图/最短反例 |

另按用户点名的 skillify 路线安装 `yeachan-heo/oh-my-claudecode@skillify`。只借用其“把已验证流程固化为 skill”的质量门槛；其 Claude/OMC 专属存储路径不适用本项目，F 交付固定写入各 worktree 的 `.codex/skills/verify-openapi-*/SKILL.md`。

`npx skills add` 对 PromptScript 目标报告“不支持全局安装”，但三个技能均已成功复制到 Codex 可读的 `~/.agents/skills/`，并且本仓库 F 任务规格直接引用这些文件；该 PromptScript 失败不影响本项目。

## 项目接入

- 00 新增 `tasks/FORMAL.md` 与 F01—F05 任务说明。
- ROADMAP/ACCEPTANCE 已把全部 F 任务设为交叉审核前的硬门槛。
- 每个 F 任务必须实际运行模型检查、做失败注入、按 create-formal + skillify 质量门槛生成项目本地 `.codex/skills/verify-openapi-*/SKILL.md`，并用 statechart 规则输出 Mermaid 状态图。
