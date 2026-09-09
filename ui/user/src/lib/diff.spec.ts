import { normalizeManifestsForDiff, stripManifestMetadata } from './diff';
import { describe, expect, it } from 'vitest';

describe('normalizeManifestsForDiff', () => {
	it('ignores upgrade notes without changing the original manifest', () => {
		const catalogManifest = {
			name: 'Server',
			upgradeNote: 'Review settings before upgrading.'
		};
		const deployedManifest = {
			name: 'Server'
		};

		const [normalizedCatalog, normalizedDeployed] = normalizeManifestsForDiff(
			catalogManifest,
			deployedManifest
		);

		expect(normalizedCatalog).toEqual(normalizedDeployed);
		expect(normalizedCatalog).not.toHaveProperty('upgradeNote');
		expect(catalogManifest.upgradeNote).toBe('Review settings before upgrading.');
	});

	it('treats flattened catalog configuration as equivalent to deployed configuration', () => {
		const catalogManifest = {
			name: 'Server',
			runtime: 'remote' as const,
			config: [
				{
					key: 'TOKEN',
					name: 'Token',
					description: '',
					required: true,
					sensitive: true,
					value: '',
					usage: 'env' as const
				},
				{
					key: 'Authorization',
					name: 'Authorization',
					description: '',
					required: true,
					sensitive: true,
					value: '',
					usage: 'header' as const
				},
				{
					key: 'CONFIG',
					name: 'Config',
					description: '',
					required: false,
					sensitive: false,
					value: '',
					usage: 'dynamicFile' as const
				}
			]
		};
		const deployedManifest = { ...catalogManifest, repoURL: 'https://example.com/repo' };

		const [normalizedCatalog, normalizedDeployed] = normalizeManifestsForDiff(
			catalogManifest,
			deployedManifest
		);

		expect(normalizedCatalog).toEqual(normalizedDeployed);
	});

	it('preserves configuration usage changes as drift', () => {
		const currentManifest = {
			name: 'Server',
			runtime: 'remote' as const,
			config: [
				{
					key: 'TOKEN',
					name: 'Token',
					description: '',
					required: true,
					sensitive: true,
					value: '',
					usage: 'env' as const
				}
			]
		};
		const nextManifest = {
			...currentManifest,
			config: [{ ...currentManifest.config[0], usage: 'header' as const }]
		};

		const [normalizedCurrent, normalizedNext] = normalizeManifestsForDiff(
			currentManifest,
			nextManifest
		);

		expect(normalizedCurrent).not.toEqual(normalizedNext);
	});

	it('strips admin-added metadata from flattened configuration', () => {
		const manifest = stripManifestMetadata({
			config: [
				{
					key: 'TOKEN',
					usage: 'env',
					secretBinding: { adminAdded: true }
				}
			]
		});

		expect(manifest).toEqual({
			config: [{ key: 'TOKEN', usage: 'env', secretBinding: {} }]
		});
	});
});
