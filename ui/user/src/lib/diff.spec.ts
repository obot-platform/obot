import { normalizeManifestsForDiff } from './diff';
import { describe, expect, it } from 'vitest';

describe('normalizeManifestsForDiff', () => {
	it('ignores upgrade notes on root and composite component manifests', () => {
		const catalogManifest = {
			name: 'Composite server',
			upgradeNote: 'Review settings before upgrading.',
			compositeConfig: {
				componentServers: [
					{
						catalogEntryID: 'component',
						manifest: {
							name: 'Component server',
							upgradeNote: 'Back up component settings.'
						}
					}
				]
			}
		};
		const deployedManifest = {
			name: 'Composite server',
			compositeConfig: {
				componentServers: [
					{
						catalogEntryID: 'component',
						manifest: { name: 'Component server' }
					}
				]
			}
		};

		const [normalizedCatalog, normalizedDeployed] = normalizeManifestsForDiff(
			catalogManifest,
			deployedManifest
		);

		expect(normalizedCatalog).toEqual(normalizedDeployed);
		expect(normalizedCatalog).not.toHaveProperty('upgradeNote');
		expect(normalizedCatalog).not.toHaveProperty(
			'compositeConfig.componentServers.0.manifest.upgradeNote'
		);
	});
});
