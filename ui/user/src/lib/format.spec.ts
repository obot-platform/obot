import {
	AGENTS_HOME_CLIENT_LABEL,
	canonicalDeviceClientNameFilter,
	formatDeviceClient
} from './format';
import { describe, expect, it } from 'vitest';

describe('formatDeviceClient', () => {
	it('uses branded names for supported local-agent IDs', () => {
		expect(formatDeviceClient('claude_code')).toBe('Claude Code');
		expect(formatDeviceClient('codex')).toBe('Codex');
		expect(formatDeviceClient('cursor')).toBe('Cursor');
		expect(formatDeviceClient('vscode')).toBe('VS Code');
		expect(formatDeviceClient('workbuddy')).toBe('WorkBuddy');
		expect(formatDeviceClient('opencode')).toBe('OpenCode');
		expect(formatDeviceClient('zcode')).toBe('ZCode');
	});

	it('also formats scanner/UI aliases', () => {
		expect(formatDeviceClient('claude-code')).toBe('Claude Code');
		expect(formatDeviceClient('open-code')).toBe('OpenCode');
	});

	it('preserves unknown client names', () => {
		expect(formatDeviceClient('custom-client')).toBe('custom-client');
	});

	it('keeps the shared agents-home scope label', () => {
		expect(formatDeviceClient('multi', '~/.agents/skills/example')).toBe(AGENTS_HOME_CLIENT_LABEL);
	});
});

describe('canonicalDeviceClientNameFilter', () => {
	it('maps visible branded names to inventory API values', () => {
		expect(canonicalDeviceClientNameFilter('Claude Code')).toBe('claude-code');
		expect(canonicalDeviceClientNameFilter('VS Code')).toBe('vscode');
		expect(canonicalDeviceClientNameFilter('OpenCode')).toBe('opencode');
		expect(canonicalDeviceClientNameFilter('WorkBuddy')).toBe('workbuddy');
		expect(canonicalDeviceClientNameFilter('ZCode')).toBe('zcode');
	});

	it('preserves raw and unknown search values', () => {
		expect(canonicalDeviceClientNameFilter('claude-code')).toBe('claude-code');
		expect(canonicalDeviceClientNameFilter('custom-client')).toBe('custom-client');
		expect(canonicalDeviceClientNameFilter('  ')).toBe('');
	});
});
