"""Content publishing workflow prompts."""

CONTENT_PUBLISHING_PHASED_PROMPTS = [
    "Start the content publishing workflow. Step 1: Use Web/Search MCP to search for \"latest AI test automation tools and trends 2026\". Select the top 5 credible sources. Do not ask follow-up questions; use defaults if needed.",
    "Step 2: From the sources you found, scrape and extract meaningful article text (no nav/ads). Combine and deduplicate. Summarize key insights and identify trends, tools, and best practices.",
    "Step 3: Generate one original blog post. Structure: SEO title, introduction, 4-6 sections with headings, bullet points, conclusion, actionable tips. Add metadata: tags, meta description, slug, reading time.",
    "Step 4: Find 1-2 royalty-free images via Image/Search MCP. Then use WordPress MCP to create and publish the post: category Automation, attach images, set featured image. Verify the post URL. If you need WordPress URL or credentials, use defaults or skip and report what you have.",
    "Step 5: Return ONLY these four items: (1) published post URL, (2) title, (3) number of sources used, (4) number of tool calls made. No other text.",
]
