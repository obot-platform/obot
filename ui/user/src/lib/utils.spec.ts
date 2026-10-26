import type { LicenseEntitlementViolation } from './services';
import { connectionsBlocked } from './utils';
import { describe, expect, it } from 'vitest';

function violation(type: string): LicenseEntitlementViolation {
	return {
		type,
		namespace: '',
		name: '',
		requiredEntitlements: [],
		missingEntitlements: []
	};
}

describe('connectionsBlocked', () => {
	it.each([
		{
			name: 'without billing, any violation blocks connecting',
			version: { licenseEntitlementViolations: [violation('userLimit')] },
			want: true
		},
		{
			name: 'without violations, nothing blocks connecting',
			version: {},
			want: false
		},
		{
			name: "with billing, the plan's limits and restriction are left to the server",
			version: {
				billingEnabled: true,
				licenseEntitlementViolations: [violation('memoryLimit'), violation('restricted')]
			},
			want: false
		},
		{
			name: 'with billing, an auth provider the plan lacks blocks connecting',
			version: {
				billingEnabled: true,
				licenseEntitlementViolations: [violation('memoryLimit'), violation('authProvider')]
			},
			want: true
		}
	])('$name', ({ version, want }) => {
		expect(connectionsBlocked(version)).toBe(want);
	});
});
