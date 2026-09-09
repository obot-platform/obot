export const AUTH_SCOPE_DESCRIPTION =
	'Agent Identities are API keys with policy-based permissions. Each identity defines which MCP servers, Skills, LLMs, and Obot API capabilities an agent can access.\n\n' +
	"When you create an identity, Obot issues an API key tied to those permissions. Every request made with that key is evaluated against the identity's policy, giving you centralized access control and consistent permission enforcement across agent workloads.";
