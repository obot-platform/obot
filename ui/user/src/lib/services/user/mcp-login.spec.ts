import { UserService } from '$lib/services';
import { isMcpLoginURL } from './mcp';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const originalBaseURL = UserService.baseURL;

beforeEach(() => {
	UserService.baseURL = 'https://obot.example/api';
	vi.stubEnv('DEV', false);
});

afterEach(() => {
	UserService.baseURL = originalBaseURL;
	vi.unstubAllEnvs();
});

describe('MCP CLI login URL classification', () => {
	it.each([
		'/oauth/mcp/login/ui-local-attempt',
		'https://obot.example/oauth/mcp/login/ui-local-attempt',
		'https://obot.example/prefix/oauth/mcp/login/ui-local-attempt'
	])('accepts an Obot login URL: %s', (url) => {
		expect(isMcpLoginURL(url)).toBe(true);
	});

	it.each([
		'https://provider.example/oauth/mcp/login/step?state=example',
		'https://provider.example/oauth/mcp/login/step',
		'//provider.example/oauth/mcp/login/step',
		'https://obot.example@provider.example/oauth/mcp/login/step',
		'https://user:password@obot.example/oauth/mcp/login/step',
		'https://obot.example/oauth/mcp/login/attempt?state=example',
		'https://obot.example/oauth/mcp/login/attempt#fragment',
		'https://obot.example/oauth/mcp/login/',
		'https://obot.example/oauth/mcp/login/attempt/extra',
		'https://obot.example/authorize?state=example',
		'ftp://obot.example/oauth/mcp/login/attempt',
		'http://localhost:8080/oauth/mcp/login/attempt',
		'http://['
	])('rejects a URL that is not an Obot login attempt: %s', (url) => {
		expect(isMcpLoginURL(url)).toBe(false);
	});

	it('accepts the configured development proxy target only in development', () => {
		vi.stubEnv('VITE_API_TARGET', 'https://dev.obot.example/api');
		const url = 'https://dev.obot.example/oauth/mcp/login/attempt';
		expect(isMcpLoginURL(url)).toBe(false);
		vi.stubEnv('DEV', true);
		expect(isMcpLoginURL(url)).toBe(true);
		expect(isMcpLoginURL('https://provider.example/oauth/mcp/login/attempt')).toBe(false);
	});

	it('accepts the default development proxy target', () => {
		vi.stubEnv('DEV', true);
		vi.stubEnv('VITE_API_TARGET', '');
		expect(isMcpLoginURL('http://localhost:8080/oauth/mcp/login/attempt')).toBe(true);
	});
});
