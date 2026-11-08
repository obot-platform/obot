# Feature ideas: after the provenance rework is working

Recorded 2026-11-08 from Craig. None of these have been started.

## 1. Built-in MCP servers: one for users, one for admins

**Status (2026-11-08):** first version built on this branch. `/mcp-connect/obot` (search,
request, follow requests) and `/mcp-connect/obot-admin` (list, approve into an existing or new
vMCP, deny); a Slack incoming webhook alert linking to `/mcp-server-requests/{id}`; managed
settings snippets on the Server Requests page. Still open: Slack interactive buttons (approve
from Slack itself), letting an admin edit the catalog entry before approving, and what else the
admin server should do (self-configuration).

### User MCP server
- Admins can push it to users through Claude managed settings.
- Users can search the MCP servers that are already in Obot's catalog.
- A user who finds a server on the web can ask, from whatever agent they're using, to add it to
  Obot's catalog and to a vMCP server.

### Approval flow for those requests
- The request goes to Obot, which sends the admin a Slack alert with a link.
- The link opens a page where the admin approves or denies the request.
- Approving walks the admin through the next step: add the server to an existing vMCP, or create
  a new vMCP for it.

### Admin MCP server
- Can change Obot's own configuration.
- How to use it is still undecided. It connects to the goal of a **self-improving system** and
  may need an agent running inside Obot.

## 2. Top-level "AI" chat in the UI (the "Uber agent")
- Add a top-level **AI** item with an AI icon to the left nav. It opens a chat.
- The chat can drive all of Obot: answer questions about what is happening, pull data, and change
  configuration.
- Areas of focus. Each could be a separate agent, part of the same agent, or a dashboard section:
  - Cost savings
  - Security and compliance
  - Operations at the Kubernetes level
  - Adoption
- The overall framing: the system is about **AI enablement in a governed place**.

## 3. AI data tagger (smaller; extends existing work)
- An AI looks at data flowing through the system and decides how to tag it.
- Two modes: in real time, or in batches on an interval.
- A **sample rate** for real-time mode, setting how often the tagger checks traffic.
- Note: the branch already has an optional AI content tagger (`feat(provenance): optional AI
  content tagger`). Start from that.

## 4. Discover the apps an org uses and recommend MCP servers (Vanta-style)
- Vanta, used here for SOC 2, connects to the identity provider and from there finds all of the
  company's app integrations. Research how they do it. (The name Craig gave, "Ontra", is likely a
  mistranscription of the IdP, such as Okta.)
- Obot could do the same: find the apps the org uses, then suggest something like "We have these
  MCP servers that would work well. Do you want help adding them to the catalog and configuring
  them?"
