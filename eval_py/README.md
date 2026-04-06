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
    │   ├── framework.py            # Case, Context, Result, run_all, pass_count
    │   └── cases.py                # all_cases(), workflow + blog-post runners
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
| **`OBOT_EVAL_AUTH_HEADER`** | Live API calls (if your server requires auth) | e.g. `Cookie: obot_access_token=...` or `Bearer …`. |
| **`OBOT_EVAL_API_LOG`** | Optional | If set, REST/MCP traffic is appended to that file path. |
| **`OBOT_EVAL_CONVERSATION_WORKFLOW`** | Optional | Override workflow id when using the shared runner (defaults per wrapper case). |

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
export OBOT_EVAL_AUTH_HEADER='Cookie: obot_access_token=YOUR_TOKEN'
export OPENAI_API_KEY=sk-...
python -m eval.run
```

**Windows (PowerShell)**

```powershell
cd eval_py
$env:OBOT_EVAL_BASE_URL = "http://localhost:8080"
$env:OBOT_EVAL_AUTH_HEADER = "Cookie: obot_access_token=YOUR_TOKEN"
$env:OPENAI_API_KEY = "sk-..."
.\venv\Scripts\python.exe -m eval.run
```

- **`python -m eval.run`** — all cases in `all_cases()`.
- **`python -m eval.run nanobot_python_code_review_conversation_eval`** — one case.
- **`python -m eval.run nanobot_conversation_workflows`** — the three conversation workflows only.

Artifacts are written under **`eval/data/`** (e.g. `step_eval_output.txt`, `step_eval_output_distinct.txt`, `data.json`, and per-workflow `python_review.txt`, `news.txt`, `antv_charts.txt` when applicable).

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

- **Static job** — daily schedule + every run: checkout, Python 3.11, `pip install -r eval_py/requirements.txt`, `python -m eval.agent_deepeval all`. Needs repo secret **`OPENAI_API_KEY`**.
- **Live job** — only on **`workflow_dispatch`** with **`run_live: true`**: starts `ghcr.io/obot-platform/obot:latest`, then runs three conversation cases from `eval_py`. You may need to extend the workflow with **`OBOT_EVAL_AUTH_HEADER`** (secret) if your Obot instance requires auth.

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
