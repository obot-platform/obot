import type { MCPConfigurationOption, MCPSubField, RuntimeFormData } from '$lib/services';
import { validateRuntimeForm } from './mcp';
import { describe, expect, it } from 'vitest';

function runtimeForm(options: MCPConfigurationOption[]): RuntimeFormData {
	return {
		name: 'Example server',
		shortDescription: 'Example server description',
		description: '',
		icon: '',
		categories: [],
		serverUserType: 'singleUser',
		env: [
			{
				key: 'REGION',
				name: 'Region',
				description: '',
				value: '',
				required: true,
				sensitive: false,
				options
			}
		],
		runtime: 'npx',
		npxConfig: { package: 'example-package' }
	};
}

function envIsMissing(options: MCPConfigurationOption[]) {
	return validateRuntimeForm(runtimeForm(options), 'hosted').required.env;
}

describe('validateRuntimeForm configuration options', () => {
	it('accepts non-empty options with unique values', () => {
		expect(
			envIsMissing([
				{ name: 'United States', value: 'us' },
				{ name: 'Europe', value: 'eu' }
			])
		).toBeUndefined();
	});

	it('validates options on remote catalog environment fields', () => {
		const form = runtimeForm([{ name: 'United States', value: ' ' }]);
		form.runtime = 'remote';
		form.npxConfig = undefined;
		form.remoteConfig = { fixedURL: 'https://example.com' };

		expect(validateRuntimeForm(form, 'remote').required.env).toBe(true);
	});

	it.each([
		{
			name: 'blank name',
			options: [{ name: ' ', value: 'us' }]
		},
		{
			name: 'blank value',
			options: [{ name: 'United States', value: ' ' }]
		}
	])('rejects options with a $name', ({ options }) => {
		expect(envIsMissing(options)).toBe(true);
	});

	it('reports duplicate environment option values as invalid', () => {
		const validation = validateRuntimeForm(
			runtimeForm([
				{ name: 'United States', value: 'us' },
				{ name: 'US fallback', value: 'us' }
			]),
			'hosted'
		);

		expect(validation.required.env).toBeUndefined();
		expect(validation.invalid.env).toBe(true);
	});

	it('reports duplicate remote header option values as invalid', () => {
		const form = runtimeForm([]);
		form.runtime = 'remote';
		form.npxConfig = undefined;
		form.remoteConfig = {
			fixedURL: 'https://example.com',
			headers: [
				{
					key: 'X-REGION',
					name: 'Region',
					description: '',
					value: '',
					required: true,
					sensitive: false,
					options: [
						{ name: 'United States', value: 'us' },
						{ name: 'US fallback', value: 'us' }
					]
				}
			]
		};

		const validation = validateRuntimeForm(form, 'remote');
		expect(validation.required.headers).toBeUndefined();
		expect(validation.invalid.headers).toBe(true);
	});

	it('rejects options combined with a static value or secret binding', () => {
		const withValue = runtimeForm([{ name: 'United States', value: 'us' }]);
		withValue.env[0].value = 'us';

		const withSecretBinding = runtimeForm([{ name: 'United States', value: 'us' }]);
		(withSecretBinding.env[0] as MCPSubField).secretBinding = {
			name: 'api-credentials',
			key: 'api-key'
		};

		expect(validateRuntimeForm(withValue, 'hosted').required.env).toBe(true);
		expect(validateRuntimeForm(withSecretBinding, 'hosted').required.env).toBe(true);
	});
});
