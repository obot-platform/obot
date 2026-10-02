// A SCIM bearer token that expires within this many days is shown as expiring.
export const TOKEN_EXPIRY_WARNING_DAYS = 30;

// tokenExpiryState reports whether a SCIM bearer token that expires at expiresAt has expired, or
// expires within TOKEN_EXPIRY_WARNING_DAYS, so the identity provider must be given a new one soon.
export function tokenExpiryState(
	expiresAt: string | undefined,
	now = Date.now()
): 'expired' | 'expiring' | undefined {
	if (!expiresAt) return undefined;
	const remaining = new Date(expiresAt).getTime() - now;
	if (remaining <= 0) return 'expired';
	if (remaining < TOKEN_EXPIRY_WARNING_DAYS * 24 * 60 * 60 * 1000) return 'expiring';
	return undefined;
}
