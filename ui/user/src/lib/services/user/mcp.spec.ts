import type {
	MCPConfigurationOption,
	MCPSubField,
	RuntimeFormData,
	SystemMCPServerCatalogEntryManifest
} from '$lib/services';
import {
	convertServerRuntimeFormDataToManifest,
	getManifestConfiguration,
	manifestHasSecretBindings,
	hasMissingSecretBindingConfig,
	validateRuntimeForm
} from './mcp';
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

describe('catalog configuration schema', () => {
	it('serializes all server configuration usages without legacy fields', () => {
		const form = runtimeForm([]);
		const field = form.env[0];
		form.runtime = 'remote';
		form.env = [
			field,
			{ ...field, key: 'FILE', file: true },
			{ ...field, key: 'DYNAMIC', file: true, dynamicFile: true },
			{ ...field, key: 'INPUT', interpolated: true },
			{ ...field, key: 'DYNAMIC_WITHOUT_FILE', file: false, dynamicFile: true },
			{ ...field, key: 'INTERPOLATED_FILE', file: true, dynamicFile: true, interpolated: true }
		];
		form.remoteServerConfig = {
			url: 'https://example.com',
			headers: [{ ...field, key: 'STATIC_HEADER' }]
		};
		form.multiUserConfig = { userDefinedHeaders: [{ ...field, key: 'USER_HEADER' }] };
		const { manifest } = convertServerRuntimeFormDataToManifest(form);
		expect(manifest.config?.map(({ usage }) => usage)).toEqual([
			'env',
			'file',
			'dynamicFile',
			'interpolated',
			'env',
			'interpolated',
			'header',
			'header'
		]);
		expect(manifest.config?.at(-1)).toMatchObject({ key: 'USER_HEADER', userAllowed: true });
		expect(manifest).not.toHaveProperty('env');
		expect(manifest).not.toHaveProperty('multiUserConfig');
		expect(manifest.remoteConfig).not.toHaveProperty('headers');
	});

	it('matches missing secret bindings against the configuration usage', () => {
		const field = { ...runtimeForm([]).env[0], secretBinding: { name: 'secret', key: 'token' } };
		const manifest = { config: [{ ...field, usage: 'header' }] };
		expect(hasMissingSecretBindingConfig(manifest, [field.key], [])).toBe(false);
		expect(hasMissingSecretBindingConfig(manifest, [], [field.key])).toBe(true);
	});
	it('treats omitted catalog values as user inputs, not static values', () => {
		const result = getManifestConfiguration(
			JSON.parse(
				'{"runtime":"remote","config":[{"key":"TOKEN","usage":"env"},{"key":"Authorization","usage":"header"}]}'
			)
		);
		expect(result.env[0].value).toBe('');
		expect(result.headers[0].value).toBe('');
	});

	it('maps all usages to existing configuration controls without losing metadata', () => {
		const field = runtimeForm([]).env[0];
		const config = (['env', 'header', 'file', 'dynamicFile', 'interpolated'] as const).map(
			(usage) => ({ ...field, key: usage, usage, value: 'configured', prefix: 'prefix' })
		);
		const result = getManifestConfiguration({ runtime: 'remote', config });
		expect(result.env.map((field) => field.key)).toEqual([
			'env',
			'file',
			'dynamicFile',
			'interpolated'
		]);
		expect(result.headers.map((field) => field.key)).toEqual(['header']);
		expect(result.env[1].file).toBe(true);
		expect(result.env[2].dynamicFile).toBe(true);
		expect(result.env[3].interpolated).toBe(true);
		expect(result.headers[0]).toMatchObject({ value: 'configured', prefix: 'prefix' });
	});

	it('reads flattened system catalog configuration', () => {
		const field = runtimeForm([]).env[0];
		const manifest: SystemMCPServerCatalogEntryManifest = {
			name: 'System server',
			shortDescription: '',
			description: '',
			icon: '',
			runtime: 'remote',
			config: [
				{ ...field, usage: 'env' },
				{ ...field, key: 'Authorization', usage: 'header' }
			]
		};

		expect(getManifestConfiguration(manifest)).toMatchObject({
			env: [{ key: 'REGION' }],
			headers: [{ key: 'Authorization' }]
		});
	});

	it('reads flattened deployed-server configuration and excludes per-user headers', () => {
		const field = runtimeForm([]).env[0];
		const result = getManifestConfiguration({
			runtime: 'remote',
			config: [
				{ ...field, usage: 'env' },
				{ ...field, usage: 'header' },
				{ ...field, key: 'USER_HEADER', usage: 'header', userAllowed: true }
			],
			remoteConfig: { url: 'https://example.com' }
		});
		expect(result).toMatchObject({ env: [field], headers: [field] });
		expect(getManifestConfiguration({ runtime: 'npx' })).toEqual({ env: [], headers: [] });
	});

	it('detects secret bindings on flattened catalog inputs', () => {
		const field = runtimeForm([]).env[0];
		expect(
			manifestHasSecretBindings({
				config: [{ ...field, usage: 'env', secretBinding: { name: 'credentials', key: 'token' } }]
			})
		).toBe(true);
		expect(manifestHasSecretBindings({ config: [{ ...field, usage: 'env' }] })).toBe(false);
	});
});
