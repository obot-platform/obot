"""Content publishing workflow prompts.

WordPress MCP must be configured with a user that has Editor or Administrator role
for create/post/publish to work; Application Passwords inherit the user's capabilities.
Read-only errors in the workflow usually mean the WordPress user role is too limited.
"""


CONTENT_PUBLISHING_PHASED_PROMPTS = [
    """I need a deep news briefing on the US–China trade war and tariffs.

Use the DuckDuckGo MCP server from Obot (no configuration needed). Connect to it and use both tools: search and fetch_content.

**Phase 1 — Search (up to 5 searches)**  
Run up to 5 DuckDuckGo searches, each with max_results=10. Use these queries if possible:
- US-China trade war tariffs latest news 2025
- US-China trade war tariffs analysis
- US-China trade war impact
- US China tariffs trade
- China United States trade war

If a search returns 0 results, try a shorter query (e.g. "US China tariffs", "trade war news") or skip. If you still get no results from search, use fetch_content on at least one known URL: https://en.wikipedia.org/wiki/China%E2%80%93United_States_trade_war (and any other relevant URLs you know). The goal is to have at least 2–3 sources to read.

**Phase 2 — Source selection**  
From the search results (or the fallback URL above), pick up to 6 URLs. Prefer diversity and substance. If you have fewer than 6, that is OK—use what you have.

**Phase 3 — Deep reading**  
Use fetch_content on each chosen URL (max 6). If a fetch fails, skip and continue. Stop after you have successfully loaded content from at least 2 sources. Do not keep fetching indefinitely.

**Phase 4 — Cross-reference**  
From the content you loaded, note: confirmed facts (2+ sources), conflicting or single-source claims, and key data points (stats, dates, quotes).

**Phase 5 — Final report (required)**  
You MUST produce the final report now. Do not fetch more sources. Do not add more tool calls. Write a single report with exactly these sections (use markdown headings):

## US–China Trade War Briefing

### Confirmed facts (2+ sources)
(2–4 sentences on facts that appear in multiple sources)

### Conflicting or single-source claims
(1–3 sentences on disagreements or unverified claims, if any)

### Key data points
(Stats, dates, or quotes that matter)

### Note on sources
(Which URLs/sources you used; if any fetch failed, say so briefly)

Then stop. Your reply must end with this report."""
]


# --- Conversation workflows: multi-turn with per-turn eval criteria ---
# Each turn: send prompt → get response → run DeepEval with turn-specific criteria → then send next prompt (eval-based reply).
# Used by nanobot_workflow_conversation_eval.

def get_conversation_turns(workflow_id: str) -> list[dict]:
    """Return list of turns for a conversation workflow. Each turn: prompt (str), criteria (list[str])."""
    if workflow_id == "python_code_review":
        return PYTHON_CODE_REVIEW_TURNS
    if workflow_id == "deep_news_briefing":
        return DEEP_NEWS_BRIEFING_TURNS
    if workflow_id == "antv_dual_axes_viz":
        return ANTV_DUAL_AXES_TURNS
    return []


PYTHON_CODE_REVIEW_TURNS = [
    {
        "prompt": """What is wrong with this Python code?

for i in range(5)
    print(i)""",
        "criteria": [
            "Mentions the missing colon (e.g. after range(5))",
            "Provides corrected code (for i in range(5): with indented print)",
            "Does not invent extra errors beyond the actual syntax errors",
        ],
    },
    {
        "prompt": """Great. Now modify this code so it prints only even numbers between 0 and 4 (0, 2, 4).

Start from the **fixed** version of the loop:

```python
for i in range(5):
    print(i)
```

Update the loop so that it prints only even numbers. Return the **full corrected loop**, not just an inner snippet.""",
        "criteria": [
            "Response modifies the code to print only even numbers (e.g. 0, 2, 4)",
            "Uses a valid approach: condition (e.g. i % 2 == 0) or range(0, 5, 2) or equivalent",
        ],
    },
]


DEEP_NEWS_BRIEFING_TURNS = [
    {
        # Single-turn deep news briefing, aligned with CONTENT_PUBLISHING_PHASED_PROMPTS.
        "prompt": CONTENT_PUBLISHING_PHASED_PROMPTS[0],
        "criteria": [
            "The response is a news-style briefing focused on the US–China trade war and tariffs.",
            "The response includes clearly separated sections for confirmed facts, more uncertain or conflicting claims, key data points, and a short note on sources.",
            "The content is coherent and stays on-topic; it does not drift to unrelated subjects.",
        ],
    },
]


ANTV_DUAL_AXES_TURNS = [
    {
        # Phase 1 – Dataset validation and understanding
        "prompt": """I want to create a professional business visualization using the AntV Charts MCP server.

Use the tool: generate_dual_axes_chart

OBJECTIVE:
Visualize Revenue vs Profit Margin trend for the year 2025 and generate business insights.

PHASE 1 - DATASET (Use this exact dataset)

[
  { "month": "Jan", "revenue": 120000, "profit_margin": 18 },
  { "month": "Feb", "revenue": 135000, "profit_margin": 20 },
  { "month": "Mar", "revenue": 150000, "profit_margin": 19 },
  { "month": "Apr", "revenue": 165000, "profit_margin": 22 },
  { "month": "May", "revenue": 190000, "profit_margin": 24 },
  { "month": "Jun", "revenue": 210000, "profit_margin": 26 },
  { "month": "Jul", "revenue": 230000, "profit_margin": 25 },
  { "month": "Aug", "revenue": 250000, "profit_margin": 28 },
  { "month": "Sep", "revenue": 240000, "profit_margin": 27 },
  { "month": "Oct", "revenue": 270000, "profit_margin": 30 },
  { "month": "Nov", "revenue": 300000, "profit_margin": 32 },
  { "month": "Dec", "revenue": 340000, "profit_margin": 35 }
]

Ensure:
- Data is chronologically ordered (Jan–Dec)
- No null values
- Revenue is numeric
- Profit margin is numeric (percentage value)

In this phase, confirm that the dataset is valid and ready to use. If you see any issues, describe them and propose simple fixes, but do not generate the chart yet.""",
        "criteria": [
            "Explicitly confirms that the dataset covers all 12 months from Jan to Dec with both revenue and profit_margin present.",
            "Mentions at least one basic data quality check (e.g. no nulls, numeric fields) and confirms that the dataset is suitable for charting or calls out any problems.",
        ],
    },
    {
        # Phase 2 – Chart configuration
        "prompt": """PHASE 2 - CHART CONFIGURATION (chart setup only; do not repeat Phase 1 validation)

Using the same 12-month dataset from Phase 1, describe how to configure `generate_dual_axes_chart`:

- X-axis -> month
- Left Y-axis -> revenue (column series)
- Right Y-axis -> profit_margin (line series)
- Title -> "Revenue vs Profit Margin - 2025 Trend Analysis"
- Tooltip, legend, smooth line, responsive layout, and grid lines enabled
- Revenue formatted as currency ($); profit_margin formatted as %
- Distinct colors for the column and line series; data labels on the line series

Reply with the chart mapping and formatting plan only. Do not re-validate the dataset.""",
        "criteria": [
            "Describes dual-axes chart setup with month on X, revenue on the left axis, and profit_margin on the right axis (column + line or equivalent).",
            "Mentions at least two of: chart title, currency formatting for revenue, percentage formatting for profit_margin, or distinct series colors.",
        ],
    },
    {
        # Phase 3 – Visual analysis and insights
        "prompt": """PHASE 3 - VISUAL ANALYSIS

Assume the chart has been generated correctly. Based on the revenue and profit_margin values in the dataset, provide:

1. 5 key insights from the trend.
2. Highest revenue month.
3. Highest profit margin month.
4. Correlation explanation between revenue and profit margin.
5. Identify any dip or anomaly.
6. 2 business risks.
7. 2 growth opportunities.
8. Executive summary (3–4 lines suitable for stakeholders).

Base your analysis strictly on the provided 2025 dataset; do not invent additional data.""",
        "criteria": [
            "Identifies the correct highest revenue and highest profit margin months from the dataset.",
            "Provides a plausible set of insights, risks, and opportunities that clearly reference the trends and any notable changes in revenue and profit_margin over the year.",
        ],
    },
]
