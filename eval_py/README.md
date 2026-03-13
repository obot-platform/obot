# Nanobot Workflow Evals (Python)

Python implementation of the Obot eval framework.  
All evals are **API‑driven** (HTTP + MCP), **no UI automation**.

---

## Folder structure

```text
eval_py/
├── requirements.txt
├── README.md
└── eval/
    ├── __init__.py          # Re-exports core types/helpers (Result, Case, run_all, all_cases, etc.)
    ├── run.py               # CLI entrypoint: python -m eval.run [case_name | group]
    │
    ├── core/                # Core eval types and case definitions
    │   ├── framework.py     # Result, Case, Context, run_all, run_from_env, pass_count, write_results_json
    │   └── cases.py         # All nanobot_* eval cases (lifecycle, workflows, conversation evals, etc.)
    │
    ├── clients/             # Obot REST and MCP clients
    │   ├── client.py        # HTTP client (version, projects, agents, etc.)
    │   └── mcp_client.py    # MCP gateway client (initialize, chat, events stream, SSE handling)
    │
    ├── workflow/            # Workflow-specific prompts and eval helpers
    │   ├── workflow_eval.py    # Content publishing response evaluation
    │   └── workflow_prompt.py  # Phased prompts + conversation turns (python_code_review, deep_news, antv_dual_axes_viz)
    │
    ├── helper/              # Utilities shared across evals
    │   ├── api_log.py           # API request/response logging
    │   ├── event_stream_data.py # SSE helpers: write step_eval_output*.txt, make_distinct_sse, data.json appends
    │   ├── paths.py             # data_dir(), data_path() pointing into eval/data
    │   ├── eval_functions.py    # (legacy helpers, not used in current 3 workflows)
    │   └── conversation_planner.py # (not used by current eval cases)
    │
    ├── data/                # Eval data and logs (git-ignored except fixtures)
    │   ├── python_review_1.json       # Static DeepEval fixture for python code review
    │   ├── deep_news_briefing_1.json  # Static DeepEval fixture for deep news briefing
    │   ├── antv_charts_1.json         # Static DeepEval fixture for AntV dual-axes
    │   ├── python_review.txt          # Latest distinct SSE for python review conversation eval
    │   ├── news.txt                   # Latest distinct SSE for deep news conversation eval
    │   ├── antv_charts.txt            # Latest distinct SSE for AntV conversation eval
    │   ├── step_eval_output.txt       # Multi-phase raw SSE + trajectory for latest workflow eval
    │   ├── step_eval_output_distinct.txt # Deduplicated variant of the above
    │   ├── eval-api-log.txt           # Optional HTTP/MCP API log (if OBOT_EVAL_API_LOG is set)
    │   └── data.json                  # Accumulated event_stream_responses (SSE snapshots per case/phase)
    │
    ├── tests/
    │   └── test_eval.py      # Pytest coverage for core cases and helpers
    │
    └── docs/
        └── STEP_EVAL_FLOW.md # Details of the content-publishing step eval flow
```

---

## Setup

```bash
cd eval_py
pip install -r requirements.txt
```

---

## Running evals

All commands below should be run from the `eval_py` directory.

### Run all configured eval cases

```bash
cd eval_py
export OBOT_EVAL_BASE_URL=http://localhost:8080
export OBOT_EVAL_AUTH_HEADER="Cookie: obot_access_token=..."
python -m eval.run
```

- Discovers all cases from `eval.core.cases.all_cases()` and runs them against the configured Obot instance.
- Prints a summary for each case with pass/fail, duration, and message.

### Run a single case

```bash
python -m eval.run nanobot_workflow_content_publishing_step_eval
```

`case_name` must match one of the names returned by `all_cases()` (e.g. `nanobot_lifecycle`, `nanobot_launch`, `nanobot_list_and_filter`, `nanobot_mock_tool_output`, etc.).

---

## Conversation workflows (3 key test cases)

On top of the original Go eval cases, the Python framework adds **three conversation evals** that exercise multi‑turn workflows and evaluate each turn with DeepEval:

- `nanobot_python_code_review_conversation_eval`
  - Workflow: `python_code_review`
  - Turns:
    1. “What is wrong with this Python code? …” (missing colon).
    2. “Now modify it to print only even numbers.”
  - Each turn is sent via MCP, SSE is captured, and DeepEval is run with turn‑specific criteria.

- `nanobot_deep_news_briefing_conversation_eval`
  - Workflow: `deep_news_briefing`
  - Single‑prompt deep news briefing on the US–China trade war and tariffs.
  - Uses the DuckDuckGo MCP (search + fetch_content), then evaluates the final report structure (confirmed facts, conflicting claims, key data points, note on sources).

- `nanobot_antv_dual_axes_conversation_eval`
  - Workflow: `antv_dual_axes_viz`
  - Phases:
    1. Dataset validation.
    2. Dual‑axes chart configuration.
    3. Visual analysis & business insights.
  - Each phase has its own criteria (data quality, chart config, analysis quality) checked via DeepEval.

### Run all three conversation workflows together

There is a convenience **group name** handled by `eval.run`:

```bash
cd eval_py
export OBOT_EVAL_BASE_URL=http://localhost:8080
export OBOT_EVAL_AUTH_HEADER="Cookie: obot_access_token=..."
python -m eval.run nanobot_conversation_workflows
```

This will:

- Run:
  - `nanobot_python_code_review_conversation_eval`
  - `nanobot_deep_news_briefing_conversation_eval`
  - `nanobot_antv_dual_axes_conversation_eval`
- For each:
  - Open an MCP session via `clients.mcp_client`.
  - For each turn, call `chat-with-nanobot`, stream `/api/events/<session_id>`, and capture response text + `raw_sse`.
  - Immediately run `agent_deepeval_generic.run_deepeval_for_turn` for that turn.
  - Append the SSE snapshot to `data.json` and update:
    - `eval/data/step_eval_output.txt`
    - `eval/data/step_eval_output_distinct.txt`
    - and write **distinct SSE per test case** into:
      - `eval/data/python_review.txt`
      - `eval/data/news.txt`
      - `eval/data/antv_charts.txt`

> The JSON fixtures (`python_review_1.json`, `deep_news_briefing_1.json`, `antv_charts_1.json`) are **static** and are not modified by these runs.

---

## Static DeepEval runs (no live Obot required)

For deterministic, offline evaluation you can use the static JSON fixtures under `eval/data` with `agent_deepeval.py`:

```bash
cd eval_py
# Python code review (static)
venv\Scripts\python.exe -X utf8 -c "from eval.agent_deepeval import run_deepeval_for_python_review_static; run_deepeval_for_python_review_static('python_review_1')"

# Deep news briefing (static)
venv\Scripts\python.exe -X utf8 -c "from eval.agent_deepeval import run_deepeval_for_static_case; run_deepeval_for_static_case('deep_news_briefing_1')"

# AntV dual-axes (static)
venv\Scripts\python.exe -X utf8 -c "from eval.agent_deepeval import run_deepeval_for_antv_static; run_deepeval_for_antv_static('antv_charts_1')"
```

These runs:

- Load the appropriate `*_1.json` fixture (user prompt, final report, tool_calls, search_queries).
- Build a DeepEval `LLMTestCase` + metrics for that workflow.
- Print a metrics summary (scores, thresholds, reasons).

---

## API log

To log all REST and MCP traffic used by evals:

```bash
export OBOT_EVAL_API_LOG="eval/data/eval-api-log.txt"
python -m eval.run nanobot_conversation_workflows
```

- `core/framework.run_all()` initializes the log via `helper.api_log.init_api_log(path)` and closes it afterwards.
- `clients/client.py` and `clients/mcp_client.py` log each request/response with:
  - method, URL, status code
  - truncated request/response bodies (UTF‑8 safe).

---

## Event stream and long-running tasks

For long‑running workflows that use tools (especially `nanobot_deep_news_briefing_single_prompt_eval` and the conversation workflows):

- The client opens `/api/events/<session_id>` and consumes SSE.
- If no events arrive for a long time (e.g. tool calls are slow), some proxies/load balancers may close idle connections after ~60s.
- The eval client:
  - Uses `expected_prompt=None` for turn 0 to tolerate prompt echo differences.
  - Uses `expected_prompt=<prompt>` for later turns so SSE slices are correctly scoped per turn.
- If `raw_sse` is empty but the same prompt works in the Obot UI, the server should consider sending periodic SSE comments (keep‑alives) so the connection is not dropped mid‑eval.

---
# Nanobot Workflow Evals (Python)

Python port of the Obot eval framework. API-driven, no UI automation.

## Folder structure

```
eval_py/
├── requirements.txt
├── README.md
└── eval/
    ├── __init__.py       # Re-exports from core (framework, cases, run_all, etc.)
    ├── run.py            # CLI: python -m eval.run
    ├── mockmcp/          # In-process mock MCP server for tests
    ├── core/             # Eval framework and case definitions
    │   ├── framework.py  # Run evals, Result, Context
    │   └── cases.py      # Case definitions and run functions
    ├── clients/          # Obot and MCP clients
    │   ├── client.py     # HTTP client (version, projects, agents)
    │   └── mcp_client.py # MCP gateway client (initialize, chat, events stream)
    ├── workflow/         # Workflow evals
    │   ├── workflow_eval.py   # Content publishing response evaluation
    │   └── workflow_prompt.py
    ├── helper/           # Utilities
    │   ├── api_log.py    # API request/response logging
    │   ├── eval_functions.py
    │   └── paths.py      # data_dir(), data_path()
    ├── tests/            # Pytest tests
    │   └── test_eval.py
    ├── data/             # Data files (e.g. data.json)
    └── docs/             # Dev documentation
        └── STEP_EVAL_FLOW.md
```

## Setup

```bash
cd eval_py
pip install -r requirements.txt
```

## Run all evals

```bash
export OBOT_EVAL_BASE_URL=http://localhost:8080
export OBOT_EVAL_AUTH_HEADER="Cookie: obot_access_token=..."
python -m eval.run
```

Run from eval_py directory so `eval` package is found.

## Run mock tool test only (no Obot)

```bash
cd eval_py
python -m pytest eval/tests/test_eval.py -v -k mock_tool
```

## Cases

Same as Go eval: `nanobot_lifecycle`, `nanobot_launch`, `nanobot_list_and_filter`, `nanobot_graceful_failure`, `nanobot_version_flag`, `nanobot_mock_tool_output`, `nanobot_workflow_content_publishing_eval`, `nanobot_workflow_content_publishing_step_eval`, plus three **conversation workflows**:

- `nanobot_python_code_review_conversation_eval` – multi-turn Python code review (missing colon, then “only even numbers”) with per-turn DeepEval criteria.
- `nanobot_deep_news_briefing_conversation_eval` – deep news briefing on US–China trade war (search + fetch_content) evaluated as a single conversation turn.
- `nanobot_antv_dual_axes_conversation_eval` – three-phase AntV dual-axes workflow (dataset validation, chart config, visual analysis) with per-phase DeepEval metrics.

### Run grouped conversation workflows

There is a convenience group in `eval/run.py` that runs all three conversation workflows in one command:

```bash
cd eval_py
export OBOT_EVAL_BASE_URL=http://localhost:8080
export OBOT_EVAL_AUTH_HEADER="Cookie: obot_access_token=..."
python -m eval.run nanobot_conversation_workflows
```

This:
- Drives each workflow via MCP (`chat-with-nanobot`) turn by turn.
- Captures event-stream SSE per turn and evaluates each with DeepEval.
- Writes:
  - Multi-phase logs to `eval/data/step_eval_output.txt` and `eval/data/step_eval_output_distinct.txt`.
  - Per-test distinct SSE logs to:
    - `eval/data/python_review.txt`
    - `eval/data/news.txt`
    - `eval/data/antv_charts.txt`

## API log

Set OBOT_EVAL_API_LOG to a file path to log all API requests/responses.

## Event stream and long-running tasks

For **nanobot_deep_news_briefing_single_prompt_eval**, the client opens the `/api/events/<session_id>` stream and waits for the assistant response. If the agent runs for a long time (e.g. tool calls for search/fetch), the server may not send any event for 60+ seconds. Many proxies and load balancers close connections that are idle (no data from server) after 60 seconds. If the eval sees "raw_sse empty" while the same prompt works in the browser, the server should send periodic SSE comments (e.g. every 15–30s) during long-running agent work so the connection is not closed. The client uses `expected_prompt=None` for the deep news single-prompt case so any assistant content is captured without requiring an exact user-message match.
