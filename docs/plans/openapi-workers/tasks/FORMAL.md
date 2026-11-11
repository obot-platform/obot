# Feature Codex 设计级形式化验证任务

本任务补齐每个 feature 设计中“测试难以覆盖、一旦出错代价高”的关键性质。必须实际运行验证工具；只写计划、伪代码或静态评审不算完成。

## 必读与工具

1. 先读 `/Users/echo/.codex/RTK.md`，所有 shell 使用 `rtk`。
2. 读本 worktree 适用 `AGENTS.md` 与 00 的 ROADMAP、CONTRACTS、ACCEPTANCE。
3. 读已安装 pstack 技能：
   - `~/.agents/skills/create-formal-verification-skill/SKILL.md`
   - `~/.agents/skills/statechart/SKILL.md`
   - `~/.agents/skills/skillify/SKILL.md`
4. 按技能说明选择工具。并发、重试、队列、租约、缓存失效和生命周期状态机默认使用 TLA+；解析器输入性质可选择有界生成/SMT，但必须说明为何比 TLA+ 更合适。

## 写集

只允许写入自己 worktree 的：

- `docs/plans/openapi-workers/PLAN.md`（追加/修订“形式化验证”章节）
- `docs/plans/openapi-workers/formal/`（模型、配置、视图、输出和非敏感证据）
- `.codex/skills/verify-openapi-<feature>/SKILL.md` 及其必要脚本

不得修改业务实现、全局配置、其他 worktree 或 00 契约。

## skillify 适配

借用 `skillify` 的质量门槛：该流程必须可重复、包含输入、步骤、成功标准、约束/坑和验证证据，且不能是五分钟可搜索到的通用说明。忽略该外部技能中的 Claude/OMC 专属 `.omc` 或 `~/.claude` 存储路径；本项目所有 F 交付仍写入上节限定的 `.codex/skills/verify-openapi-<feature>/SKILL.md`。不 commit、不 push。

## 完成标准

- 明确一个高代价性质：一句人话描述性质，一句人话描述违反时的业务伤害。
- 模型名称、变量和动作对应本 feature PLAN 的设计；列出模型抽象和未覆盖范围。
- 实际运行 TLC/其他工具，保留命令、版本、配置、状态数、结果和开始/结束时间。
- 对至少一个故意破坏的变体运行同一验证，证明验证能失败；随后恢复并通过，或保留真实发现并报告。
- 生成项目本地 `.codex/skills/verify-openapi-<feature>/SKILL.md`，包含触发条件、精确覆盖文件/设计章节、一条复跑命令、失败解读、模型同步记录和更新要求。
- 使用 `/statechart` 的规则产出 Mermaid 状态图。违反不变量时给出最短踩坑路径；通过时给出正常状态流转。图表隐藏的步骤必须说明。
- 不宣称验证覆盖未建模行为；形式化通过不代表 E2E 或部署通过。

通过 `worker_done` 交付 PLAN、模型、skill、图表和工具输出的绝对路径。
