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

Same as Go eval: nanobot_lifecycle, nanobot_launch, nanobot_list_and_filter, nanobot_graceful_failure, nanobot_version_flag, nanobot_mock_tool_output, nanobot_workflow_content_publishing_eval, nanobot_workflow_content_publishing_step_eval.

## API log

Set OBOT_EVAL_API_LOG to a file path to log all API requests/responses.

## Event stream and long-running tasks

For **nanobot_deep_news_briefing_single_prompt_eval**, the client opens the `/api/events/<session_id>` stream and waits for the assistant response. If the agent runs for a long time (e.g. tool calls for search/fetch), the server may not send any event for 60+ seconds. Many proxies and load balancers close connections that are idle (no data from server) after 60 seconds. If the eval sees "raw_sse empty" while the same prompt works in the browser, the server should send periodic SSE comments (e.g. every 15–30s) during long-running agent work so the connection is not closed. The client uses `expected_prompt=None` for the deep news single-prompt case so any assistant content is captured without requiring an exact user-message match.
