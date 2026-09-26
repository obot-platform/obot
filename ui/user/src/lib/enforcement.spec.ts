import { agentLabel, auditClientFilterLabel, localAgentProviderLabel } from './enforcement';
import { describe, expect, it } from 'vitest';

describe('agentLabel', () => {
	it('labels WorkBuddy and OpenCode audit events', () => {
		expect(agentLabel('workbuddy')).toBe('WorkBuddy');
		expect(agentLabel('opencode')).toBe('OpenCode');
	});
});

describe('localAgentProviderLabel', () => {
	it('uses branded names for known local-agent providers', () => {
		expect(localAgentProviderLabel('workbuddy')).toBe('WorkBuddy');
		expect(localAgentProviderLabel('opencode')).toBe('OpenCode');
		expect(localAgentProviderLabel('zcode')).toBe('ZCode');
	});

	it('preserves unknown client/provider values', () => {
		expect(localAgentProviderLabel('custom-client')).toBe('custom-client');
	});
});

describe('auditClientFilterLabel', () => {
	it('keeps raw unified client IDs visible next to a brand label', () => {
		expect(auditClientFilterLabel('claude-code')).toBe('claude-code · Claude Code');
		expect(auditClientFilterLabel('claude_code')).toBe('claude_code · Claude Code');
	});

	it('does not rewrite arbitrary MCP client names', () => {
		expect(auditClientFilterLabel('custom-client')).toBe('custom-client');
	});
});
