"""POST an MCP elicitation reply (same JSON-RPC shape as the nanobot UI).

All secrets must come from the environment — do not pass credentials on the command line.

Required env:
  OBOT_EVAL_MCP_URL — full MCP gateway URL, e.g. http://127.0.0.1:8080/mcp-connect/ms1<agentId>
  OBOT_EVAL_MCP_SESSION_ID — ``Mcp-Session-Id`` from your MCP session
  OBOT_EVAL_ELICITATION_ID — numeric id from SSE ``id:`` before ``elicitation/create``

Optional:
  OBOT_EVAL_AUTH_HEADER — same as other evals (Bearer … or Cookie: …)

For AWS catalog elicitation (form), set at least:
  AWS_ACCESS_KEY_ID
  AWS_SECRET_ACCESS_KEY

Optional:
  AWS_REGION (default us-east-1)
  AWS_SESSION_TOKEN (default empty)

Usage (from eval_py)::

    python -m eval.reply_elicitation

Exit 0 on HTTP 204/202; otherwise 1.
"""
from __future__ import annotations

import json
import os
import sys

from .clients.mcp_client import MCPClient


def main() -> int:
    mcp_url = (os.environ.get("OBOT_EVAL_MCP_URL") or "").strip()
    session_id = (os.environ.get("OBOT_EVAL_MCP_SESSION_ID") or "").strip()
    elic_id = (os.environ.get("OBOT_EVAL_ELICITATION_ID") or "").strip()
    auth = (os.environ.get("OBOT_EVAL_AUTH_HEADER") or "").strip()

    if not mcp_url or not session_id or not elic_id:
        print(
            "Missing OBOT_EVAL_MCP_URL, OBOT_EVAL_MCP_SESSION_ID, or OBOT_EVAL_ELICITATION_ID",
            file=sys.stderr,
        )
        return 1

    ak = os.environ.get("AWS_ACCESS_KEY_ID", "").strip()
    sk = os.environ.get("AWS_SECRET_ACCESS_KEY", "").strip()
    if not ak or not sk:
        print(
            "Missing AWS_ACCESS_KEY_ID or AWS_SECRET_ACCESS_KEY in environment",
            file=sys.stderr,
        )
        return 1

    region = os.environ.get("AWS_REGION", "us-east-1").strip() or "us-east-1"
    token = os.environ.get("AWS_SESSION_TOKEN", "").strip()

    content: dict[str, str] = {
        "AWS_ACCESS_KEY_ID": ak,
        "AWS_SECRET_ACCESS_KEY": sk,
        "AWS_REGION": region,
        "AWS_SESSION_TOKEN": token,
    }
    result = {"action": "accept", "content": content}

    client = MCPClient(mcp_url, auth)
    status, body = client.elicitation_reply(session_id, elic_id, result)
    print("HTTP %s" % status)
    if body:
        try:
            print(json.dumps(json.loads(body.decode("utf-8")), indent=2))
        except Exception:
            print(body.decode("utf-8", errors="replace"))
    if status in (204, 202):
        return 0
    return 1


if __name__ == "__main__":
    sys.exit(main())
