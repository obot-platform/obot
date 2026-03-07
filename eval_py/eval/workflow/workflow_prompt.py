"""Content publishing workflow prompts.

WordPress MCP must be configured with a user that has Editor or Administrator role
for create/post/publish to work; Application Passwords inherit the user's capabilities.
Read-only errors in the workflow usually mean the WordPress user role is too limited.
"""

# CONTENT_PUBLISHING_PHASED_PROMPTS = [
#     "Start the content publishing workflow. Step 1: Use Web/Search MCP to search for \"latest AI test automation tools and trends 2026\". Select the top 5 credible sources. Do not ask follow-up questions; use defaults if needed.",
#     "Step 2: From the sources you found, scrape and extract meaningful article text (no nav/ads). Combine and deduplicate. Summarize key insights and identify trends, tools, and best practices.",
#     "Step 3: Generate one original blog post. Structure: SEO title, introduction, 4-6 sections with headings, bullet points, conclusion, actionable tips. Add metadata: tags, meta description, slug, reading time.",
#     "Step 4: Find 1-2 royalty-free images via Image/Search MCP. Then use WordPress MCP to create and publish the post: category Automation, attach images, set featured image. Verify the post URL. If you need WordPress URL or credentials, use defaults or skip and report what you have.",
#     "Step 5: Return ONLY these four items: (1) published post URL, (2) title, (3) number of sources used, (4) number of tool calls made. No other text.",
# ]

# CONTENT_PUBLISHING_PHASED_PROMPTS = [
#     """I need a deep news briefing on the US–China trade war and tariffs.

# Use the DuckDuckGo MCP server from Obot (no configuration needed). Connect to it and use both tools: search and fetch_content.

# **Phase 1 — Search (up to 5 searches)**  
# Run up to 5 DuckDuckGo searches, each with max_results=10. Use these queries if possible:
# - US-China trade war tariffs latest news 2025
# - US-China trade war tariffs analysis
# - US-China trade war impact
# - US China tariffs trade
# - China United States trade war

# If a search returns 0 results, try a shorter query (e.g. "US China tariffs", "trade war news") or skip. If you still get no results from search, use fetch_content on at least one known URL: https://en.wikipedia.org/wiki/China%E2%80%93United_States_trade_war (and any other relevant URLs you know). The goal is to have at least 2–3 sources to read.

# **Phase 2 — Source selection**  
# From the search results (or the fallback URL above), pick up to 6 URLs. Prefer diversity and substance. If you have fewer than 6, that is OK—use what you have.

# **Phase 3 — Deep reading**  
# Use fetch_content on each chosen URL (max 6). If a fetch fails, skip and continue. Stop after you have successfully loaded content from at least 2 sources. Do not keep fetching indefinitely.

# **Phase 4 — Cross-reference**  
# From the content you loaded, note: confirmed facts (2+ sources), conflicting or single-source claims, and key data points (stats, dates, quotes).

# **Phase 5 — Final report (required)**  
# You MUST produce the final report now. Do not fetch more sources. Do not add more tool calls. Write a single report with exactly these sections (use markdown headings):

# ## US–China Trade War Briefing

# ### Confirmed facts (2+ sources)
# (2–4 sentences on facts that appear in multiple sources)

# ### Conflicting or single-source claims
# (1–3 sentences on disagreements or unverified claims, if any)

# ### Key data points
# (Stats, dates, or quotes that matter)

# ### Note on sources
# (Which URLs/sources you used; if any fetch failed, say so briefly)

# Then stop. Your reply must end with this report."""
# ]

# CONTENT_PUBLISHING_PHASED_PROMPTS = [
#     """Create and execute a fully automated content publishing workflow. Do not ask follow-up questions. If any step fails, retry or choose the next best alternative automatically.
# Goal:
#  Research the latest trends in AI test automation, generate a high-quality blog post, and publish it to my WordPress site with proper structure, SEO, and media.

# Steps to perform
# 1. Use the Web/Search MCP to search for:
#  "latest AI test automation tools and trends 2026"
# 2. Select the top 5 credible sources (ignore ads or low-quality blogs).
# 3. For each source:
# scrape the page content
# extract only meaningful article text
# remove navigation/ads
# if scraping fails, retry once or move to the next source
# 4. Combine the extracted information and:
# deduplicate repeated information
# summarize key insights
# identify trends, tools, and best practices
# generate one original article (not copy-paste)
# 5. Structure the blog post with:
# SEO title
# introduction
# 4–6 sections with headings
# bullet points
# comparison table (if tools are mentioned)
# conclusion
# actionable tips
# 6. Generate metadata:
# tags
# meta description
# slug
# estimated reading time
# 7. Use an Image/Search MCP to find 1–2 relevant royalty-free images.
#  Upload them to WordPress and embed them in the post.
# 8. Use WordPress MCP to:
# create a new post
# status = published
# category = Automation
# attach images
# add metadata
# set featured image
# 9. After publishing:
# verify the post URL is accessible
# confirm content is present
# retry publish if necessary

# Constraints
# * Minimize unnecessary tool calls
# * Avoid duplicate scraping
# * Retry failures intelligently
# * Use the fewest steps possible
# * Stop only after successful publish


# Output
# Return ONLY:
# * published post URL
# * title
# * number of sources used
# * number of tool calls made """
# ]

CONTENT_PUBLISHING_PHASED_PROMPTS = [
    """I want to create a professional business visualization using the AntV Charts MCP server.

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


PHASE 2 — CHART CONFIGURATION


Generate a Dual Axes Chart with:

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

PHASE 3 — VISUAL ANALYSIS


After generating the chart, analyze it and provide:

1. 5 key insights from the trend
2. Highest revenue month
3. Highest profit margin month
4. Correlation explanation between revenue and profit margin
5. Identify any dip or anomaly
6. 2 business risks
7. 2 growth opportunities
8. Executive summary (3–4 lines suitable for stakeholders)

IMPORTANT:
Deliver the chart first.
Then provide the analysis clearly structured with headings."""
]
