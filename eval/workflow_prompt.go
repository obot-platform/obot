package eval

// ContentPublishingWorkflowPrompt is the sample prompt for the "content publishing workflow"
// eval. Run this in nanobot; capture the final response and pass it to
// EvaluateContentPublishingResponse or the nanobot_workflow_content_publishing_eval case.
const ContentPublishingWorkflowPrompt = `Create and execute a fully automated content publishing workflow. Do not ask follow-up questions. If any step fails, retry or choose the next best alternative automatically.
Goal:
 Research the latest trends in AI test automation, generate a high-quality blog post, and publish it to my WordPress site with proper structure, SEO, and media.

Steps to perform
1. Use the Web/Search MCP to search for:
 "latest AI test automation tools and trends 2026"
2. Select the top 5 credible sources (ignore ads or low-quality blogs).
3. For each source:
scrape the page content
extract only meaningful article text
remove navigation/ads
if scraping fails, retry once or move to the next source
4. Combine the extracted information and:
deduplicate repeated information
summarize key insights
identify trends, tools, and best practices
generate one original article (not copy-paste)
5. Structure the blog post with:
SEO title
introduction
4–6 sections with headings
bullet points
comparison table (if tools are mentioned)
conclusion
actionable tips
6. Generate metadata:
tags
meta description
slug
estimated reading time
7. Use an Image/Search MCP to find 1–2 relevant royalty-free images.
 Upload them to WordPress and embed them in the post.
8. Use WordPress MCP to:
create a new post
status = published
category = Automation
attach images
add metadata
set featured image
9. After publishing:
verify the post URL is accessible
confirm content is present
retry publish if necessary

Constraints

Minimize unnecessary tool calls
Avoid duplicate scraping
Retry failures intelligently
Use the fewest steps possible
Stop only after successful publish


Output
Return ONLY:

published post URL
title
number of sources used
number of tool calls made`
