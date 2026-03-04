"""Content publishing workflow prompts.

WordPress MCP must be configured with a user that has Editor or Administrator role
for create/post/publish to work; Application Passwords inherit the user's capabilities.
Read-only errors in the workflow usually mean the WordPress user role is too limited.
"""


CONTENT_PUBLISHING_PHASED_PROMPTS = [
    """I need a concise news briefing on the US–China trade war and tariffs.

Use the DuckDuckGo MCP server from Obot (no configuration needed). Connect to it and use both tools: search and fetch_content.

You can use **up to 3 searches** with short queries like:
- US China trade war tariffs latest
- US China tariffs trade impact
- China United States trade war overview

From the results, pick a **small set of credible sources** (major news, analysis, or reference sites), fetch their content, and then write a single, structured briefing:

## US–China Trade War Briefing

### Confirmed facts (2+ sources)
Summarize the main, broadly agreed facts in a few sentences.

### Conflicting or single-source claims
Briefly mention any disagreements or claims that appear in only one source.

### Key data points
List the most important numbers, dates, or quotes.

### Note on sources
Name the main sources you relied on.

Do not ask the user follow-up questions. If some details are missing, make reasonable assumptions and clearly label them as such."""
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
        "prompt": "Great. Now modify it to print only even numbers.",
        "criteria": [
            "Response modifies the code to print only even numbers (e.g. 0, 2, 4)",
            "Uses a valid approach: condition (e.g. i % 2 == 0) or range(0, 5, 2) or equivalent",
        ],
    },
]


DEEP_NEWS_BRIEFING_TURNS = [
    {
        # Phase 1 – Search strategy
        "prompt": """Deep news briefing: US–China trade war and tariffs (Phase 1 — Search).

Use the DuckDuckGo MCP server from Obot (no configuration needed). Connect to it and use both tools: search and fetch_content.

In this phase, run up to 5 DuckDuckGo searches, each with max_results=10. Prefer queries like:
- US-China trade war tariffs latest news 2025
- US-China trade war tariffs analysis
- US-China trade war impact
- US China tariffs trade
- China United States trade war

If a search returns 0 results, try a shorter query (e.g. "US China tariffs", "trade war news") or skip.

Your task in this turn:
- Propose and justify which 3–5 concrete queries you will run and why.
- Explain briefly how you will handle 0-result cases.
- Do NOT fetch or summarize articles yet, just outline and justify the search plan.""",
        "criteria": [
            "Proposes at least three concrete DuckDuckGo search queries focused on the US–China trade war and tariffs.",
            "Mentions a reasonable fallback strategy for 0-result searches (e.g. shortening queries or trying broader terms).",
        ],
    },
    {
        # Phase 2 – Source selection and deep reading plan
        "prompt": """Deep news briefing: US–China trade war and tariffs (Phase 2 — Sources and reading plan).

Assume Phase 1 searches have completed and produced a mix of results.

Your task in this turn:
- Describe what kinds of sources you will pick (up to 6 URLs) and why (e.g. major news outlets, think tanks, government or multilateral reports, etc.).
- Explain criteria for deciding which results to ignore (e.g. low-quality blogs, content farms, duplicates).
- Outline how you will use fetch_content on each chosen URL, including when to stop (after at least 2 successful loads, not fetching indefinitely).
- Do NOT write the final report yet; focus on selection and deep reading strategy.""",
        "criteria": [
            "Explains a clear, reasonable strategy for selecting up to six diverse and credible sources from search results.",
            "Mentions stopping conditions for fetch_content (e.g. after at least two successful loads, avoid infinite fetching).",
        ],
    },
    {
        # Phase 3 – Cross-reference and evidence structure
        "prompt": """Deep news briefing: US–China trade war and tariffs (Phase 3 — Cross-reference and evidence).

Assume you have successfully loaded content from at least two of your chosen sources.

Your task in this turn:
- Explain how you will distinguish between confirmed facts (appearing in 2+ sources) and single-source or conflicting claims.
- Describe how you will track and use key data points (stats, dates, quotes) so they can be cited later.
- Specify how you will handle disagreements between sources in the final briefing (e.g. flagging uncertainty, noting which side each claim comes from).
- Do NOT write the final report yet; focus on how you will cross-check and organize evidence.""",
        "criteria": [
            "Describes a concrete method for separating multi-source confirmed facts from single-source or conflicting claims.",
            "Mentions collecting specific data points (stats, dates, quotes) and how they will be used or cited in the final report.",
        ],
    },
    {
        # Phase 4 – Final report structure
        "prompt": """Deep news briefing: US–China trade war and tariffs (Phase 4 — Final report structure).

Now, based on the prior phases and assuming you have completed search, selection, reading, and cross-reference:

Write the final briefing in this exact structure (markdown headings):

## US–China Trade War Briefing

### Confirmed facts (2+ sources)
(2–4 sentences on facts that appear in multiple sources)

### Conflicting or single-source claims
(1–3 sentences on disagreements or unverified claims, if any)

### Key data points
(Stats, dates, or quotes that matter)

### Note on sources
(Which URLs/sources you used; if any fetch failed, say so briefly)

In this turn, produce the full report text. Do not describe your plan; write the briefing itself, following the sections above.""",
        "criteria": [
            "Produces a report with all four required sections and correct markdown headings.",
            "Uses content that is consistent with the topic (US–China trade war and tariffs) and clearly distinguishes confirmed facts from more uncertain claims.",
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

PHASE 1 — DATASET (Use this exact dataset)

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
        "prompt": """PHASE 2 — CHART CONFIGURATION

Now, describe exactly how you would configure the AntV generate_dual_axes_chart call using this dataset:

- X-axis → month
- Left Y-axis → revenue (Column chart)
- Right Y-axis → profit_margin (Line chart)
- Chart title → "Revenue vs Profit Margin – 2025 Trend Analysis"
- Enable tooltip
- Enable legend
- Smooth line → true
- Responsive → true
- Show grid lines

Formatting:
- Format revenue as currency ($)
- Format profit_margin as %
- Show data labels on line series
- Use distinct colors for column and line

Do NOT invent a different dataset. Focus on explaining the configuration and formatting in a way that could be passed to AntV or another charting library.""",
        "criteria": [
            "Describes a dual-axes configuration with month on X, revenue as columns on the left Y-axis, and profit_margin as a line on the right Y-axis.",
            "Mentions key formatting details such as currency formatting for revenue, percentage formatting for profit_margin, and distinct colors for the two series.",
        ],
    },
    {
        # Phase 3 – Visual analysis and insights
        "prompt": """PHASE 3 — VISUAL ANALYSIS

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
