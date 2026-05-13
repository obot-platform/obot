# Nanobot workflow evals (Python)

API-driven evals for Obot nanobot: **HTTP + MCP + SSE**, no UI automation.  
Live runs talk to a real Obot instance; static runs use JSON fixtures and DeepEval only.

---


## Folder layout (current)

```text
eval_py/
├── requirements.txt
├── README.md
├── .gitignore              # venv, __pycache__, .deepeval, etc.
└── eval/
    ├── __init__.py
    ├── run.py                      # CLI: python -m eval.run
    ├── agent_deepeval.py           # Static + latest fixture DeepEval
    ├── agent_deepeval_generic.py   # Generic metrics on step_eval output; run_deepeval_for_turn
    ├── core/
    │   ├── framework.py            # Case, Context, Result, TurnEvalDetail, run_all, pass_count
    │   ├── cases.py                # all_cases(), workflow + blog-post runners
    │   └── run_summary.py          # eval_run_summary.json / .txt + optional GitHub job summary
    ├── clients/
    │   ├── client.py               # REST (version, projects, agents, MCP URL)
    │   └── mcp_client.py           # MCP + SSE / async response collection
    ├── workflow/
    │   └── workflow_prompt.py      # Conversation turns + CONTENT_PUBLISHING_PHASED_PROMPTS
    ├── helper/
    │   ├── api_log.py
    │   ├── event_stream_data.py
    │   └── paths.py
    └── data/                       # Fixtures *.json; generated logs (often gitignored locally)
```

---

## Prerequisites

| Requirement | Notes |
|-------------|--------|
| **Python** | **3.11+** (CI uses 3.11). On Windows, **3.12** is a good default for a venv. |
| **Dependencies** | `pip install -r requirements.txt` (includes **`deepeval`**). |
| **Live evals** | Running Obot (or URL to one), project + nanobot agent already set up. |
| **DeepEval** | Set **`OPENAI_API_KEY`** wherever you run turn-level or static DeepEval. |

---

## Setup

From the **repository root**, use the `eval_py` directory as the working directory so the `eval` package resolves.

**Unix/macOS**

```bash
cd eval_py
python3 -m venv venv
source venv/bin/activate
pip install -r requirements.txt
```

**Windows (PowerShell)**

```powershell
cd eval_py
py -3.12 -m venv venv
.\venv\Scripts\python.exe -m pip install -r requirements.txt
```

Use the venv interpreter for all commands below, e.g. `.\venv\Scripts\python.exe` on Windows or `python` after `activate`.

---

## Environment variables

| Variable | Required for | Purpose |
|----------|----------------|---------|
| **`OPENAI_API_KEY`** | Conversation evals (per-turn DeepEval), static DeepEval | DeepEval / judge model calls. |
| **`OBOT_EVAL_BASE_URL`** | `python -m eval.run …` | Obot API base URL, e.g. `http://localhost:8080`. |
| **`OBOT_EVAL_API_LOG`** | Optional | If set, REST/MCP traffic is appended to that file path. |
| **`OBOT_EVAL_CONVERSATION_WORKFLOW`** | Optional | Override workflow id when using the shared runner (defaults per wrapper case). |
| **`OBOT_EVAL_SKIP_AUTO_PROJECT`** | Optional | If `1` / `true`, do **not** auto-create an empty projectsv2 project + nanobot agent before evals (default: auto-create when the project list is empty). |
| **`OBOT_EVAL_PROJECT_DISPLAY_NAME`** | Optional | Display name for the auto-created project (default: `eval-py`). |
| **`OBOT_EVAL_AGENT_DISPLAY_NAME`** | Optional | Display name for the auto-created nanobot agent (default: `Eval agent`). |

### Auto project + agent (no UI, no `/launch`)

Fresh Obot instances (for example Docker in CI) often have **no projects**. Before running cases, `eval.run` calls **`POST /api/projectsv2`** and, if that project has no agents, **`POST /api/projectsv2/{id}/agents`** — the same REST surface the Nanobot UI uses. You do **not** need to open the app or call **`POST .../agents/{id}/launch`**: the MCP gateway **starts the nanobot MCP server on first connect** when eval hits `mcp-connect`.

---

## Test cases (`all_cases()`)

| Case name | What it does |
|-----------|----------------|
| `nanobot_python_code_review_conversation_eval` | Multi-turn **python_code_review** workflow; DeepEval after each turn. |
| `nanobot_deep_news_briefing_conversation_eval` | **deep_news_briefing** (single long prompt + criteria); tools via DuckDuckGo MCP in Obot. |
| `nanobot_antv_dual_axes_conversation_eval` | **antv_dual_axes_viz** three-phase AntV chart workflow; DeepEval per phase. |
| `nanobot_blog_post_elicitation_eval` | Single prompt; checks event stream for elicitation/question metadata. |

**Group** `nanobot_conversation_workflows` runs only the **first three** (not the blog-post case).

---

## Run live evals (Obot required)

```bash
cd eval_py
export OBOT_EVAL_BASE_URL=http://localhost:8080
export OPENAI_API_KEY=sk-...
python -m eval.run
```

**Windows (PowerShell)**

```powershell
cd eval_py
$env:OBOT_EVAL_BASE_URL = "http://localhost:8080"
$env:OPENAI_API_KEY = "sk-..."
.\venv\Scripts\python.exe -m eval.run
```

- **`python -m eval.run`** — all cases in `all_cases()`.
- **`python -m eval.run nanobot_python_code_review_conversation_eval`** — one case.
- **`python -m eval.run nanobot_conversation_workflows`** — the three conversation workflows only.

Step-eval and trace files are written under **`eval/data/`** (e.g. `step_eval_output.txt`, `step_eval_output_distinct.txt`, `data.json`, and per-workflow `python_review.txt`, `news.txt`, `antv_charts.txt` when applicable). See **Eval run summary** below for the aggregated pass/score report.

---

## Eval run summary

After every **`python -m eval.run …`** invocation, the CLI writes a **combined** summary for the cases that ran in that process (so one file covers the whole batch, e.g. all cases or `nanobot_conversation_workflows` only).

| Output | Path | Contents |
|--------|------|----------|
| **JSON** | `eval/data/eval_run_summary.json` | Machine-readable: `cases_total`, `cases_passed`, `case_pass_rate`, `mean_turn_score` (when DeepEval scored turns), `turns_scored`, and per-case `name`, `pass`, `duration_ms`, `message`, optional **`case_prompt`**, and **`turns`** with `turn`, `pass`, `score`, `threshold`, **`prompt`**, `reason`. |
| **Text** | `eval/data/eval_run_summary.txt` | Same information in a readable plain-text layout (prompts indented under each turn or case). |

On **GitHub Actions**, if `GITHUB_STEP_SUMMARY` is set (default on hosted runners), a short markdown block is also appended to the workflow **job summary** (pass count, pass rate, optional mean turn score, truncated prompts). Full prompts and reasons stay in **`eval_run_summary.json`** — download the **`eval-run-summary`** artifact from the workflow run to inspect that file in detail.

**Tip:** Run several conversation cases in **one** command (e.g. `nanobot_conversation_workflows`) so a single summary reflects all of them; running cases in separate processes overwrites the same two summary paths each time.

---

## Static DeepEval

Uses fixtures: `eval/data/python_review_1.json`, `deep_news_briefing_1.json`, `antv_charts_1.json`.

```bash
cd eval_py
export OPENAI_API_KEY=sk-...
python -m eval.agent_deepeval all
```

Modes for `python -m eval.agent_deepeval`: **`all`** (default), **`latest`**, **`python_review`**, **`deep_news`**, **`antv`**.

---

## Generic DeepEval on latest step-eval log

After a live run, **`eval/data/step_eval_output_distinct.txt`** contains trajectory + SSE. Process metrics (flow, tools, goal alignment, robustness):

```bash
cd eval_py
export OPENAI_API_KEY=sk-...
python -m eval.agent_deepeval_generic
```

---

## CI (GitHub Actions)

Workflow: **`.github/workflows/nanobot-python-evals.yml`**

- **Static job** — daily schedule + every run: checkout, Python 3.11, `pip install -r eval_py/requirements.txt`, `python -m eval.agent_deepeval all`. Needs repo secret **`OPENAI_API_KEY`**. This path does **not** emit `eval_run_summary.json` (that file is produced by `python -m eval.run` only).
- **Live job** — only on **`workflow_dispatch`** with **`run_live: true`**: starts `ghcr.io/obot-platform/obot:latest`, then runs **`python -m eval.run nanobot_conversation_workflows`** once (all three conversation workflows in one batch). Needs **`OPENAI_API_KEY`**. You may need to add **`OBOT_EVAL_AUTH_HEADER`** (repo secret) if your Obot instance requires auth.
- **Artifacts** — the live job uploads **`eval-run-summary`** (`eval_run_summary.json` and `eval_run_summary.txt` under `eval_py/eval/data/`) with **`if: always()`** so you can inspect results even when a case fails. The workflow run’s **Summary** tab shows the short markdown report from `GITHUB_STEP_SUMMARY`.

---

## Troubleshooting

- **`No module named 'deepeval'`** — Install deps in the same interpreter you use to run evals: `pip install -r requirements.txt` inside `eval_py` (prefer a venv).
- **`OBOT_EVAL_BASE_URL not set`** — Export/set the variable before `python -m eval.run`.
- **Empty SSE / timeouts** — Long tool runs can idle the SSE connection; see `EVENTS_STREAM_MAX_WAIT` in `mcp_client.py` and server/proxy keep-alives.

---

## API request logging

```bash
export OBOT_EVAL_API_LOG=eval/data/eval-api-log.txt
python -m eval.run nanobot_conversation_workflows
```

`run_all()` opens the log file when the variable is set and closes it after the run.
