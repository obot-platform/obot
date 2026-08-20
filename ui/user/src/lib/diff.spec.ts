import { normalizeManifestsForDiff } from './diff';
import { describe, expect, it } from 'vitest';

describe('normalizeManifestsForDiff', () => {
	it('ignores the upgrade note on the root manifest', () => {
		const catalogManifest = {
			name: 'Server',
			upgradeNote: 'Review settings before upgrading.'
		};
		const deployedManifest = { name: 'Server' };

		const [normalizedCatalog, normalizedDeployed] = normalizeManifestsForDiff(
			catalogManifest,
			deployedManifest
		);

		expect(normalizedCatalog).toEqual(normalizedDeployed);
		expect(normalizedCatalog).not.toHaveProperty('upgradeNote');
	});
});
