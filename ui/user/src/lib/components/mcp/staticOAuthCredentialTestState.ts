export type StaticOAuthCredentialTestState =
	| { status: 'idle' }
	| { status: 'pending'; clientID: string; clientSecret: string }
	| {
			status: 'succeeded';
			clientID: string;
			clientSecret: string;
			proof: string;
			expiresAt: string;
	  }
	| { status: 'failed'; failureCategory: string };

export const idleStaticOAuthCredentialTest = (): StaticOAuthCredentialTestState => ({
	status: 'idle'
});

export function beginStaticOAuthCredentialTest(
	clientID: string,
	clientSecret: string
): StaticOAuthCredentialTestState {
	return {
		status: 'pending',
		clientID,
		clientSecret
	};
}

export function succeedStaticOAuthCredentialTest(
	state: StaticOAuthCredentialTestState,
	proof: string,
	expiresAt: string
): StaticOAuthCredentialTestState {
	const expiry = Date.parse(expiresAt);
	if (state.status !== 'pending' || !proof.trim() || !Number.isFinite(expiry)) {
		return { status: 'failed', failureCategory: 'invalid_test_result' };
	}
	return { ...state, status: 'succeeded', proof, expiresAt };
}

export function failStaticOAuthCredentialTest(
	_state: StaticOAuthCredentialTestState,
	failureCategory: string
): StaticOAuthCredentialTestState {
	return { status: 'failed', failureCategory };
}

export function invalidateStaticOAuthCredentialTest(
	_state: StaticOAuthCredentialTestState
): StaticOAuthCredentialTestState {
	return idleStaticOAuthCredentialTest();
}

export function safeStaticOAuthAuthorizationURL(rawURL: string): string | undefined {
	try {
		const parsed = new URL(rawURL);
		if ((parsed.protocol !== 'https:' && parsed.protocol !== 'http:') || !parsed.hostname) {
			return undefined;
		}
		return parsed.href;
	} catch {
		return undefined;
	}
}

export function canSaveStaticOAuthCredentials(
	state: StaticOAuthCredentialTestState,
	clientID: string,
	clientSecret: string,
	now = Date.now()
): state is Extract<StaticOAuthCredentialTestState, { status: 'succeeded' }> {
	return (
		state.status === 'succeeded' &&
		state.clientID === clientID &&
		state.clientSecret === clientSecret &&
		Date.parse(state.expiresAt) > now
	);
}

type StaticOAuthCredentialGeneration = {
	configured: boolean;
	clientID?: string;
	generation?: string;
};

export async function staticOAuthSaveWasCommitted(
	current: StaticOAuthCredentialGeneration | undefined,
	clientID: string,
	proof: string
): Promise<boolean> {
	if (!current?.configured || current.clientID !== clientID || !current.generation || !proof) {
		return false;
	}
	const digest = await crypto.subtle.digest('SHA-256', new TextEncoder().encode(proof));
	const receipt = Array.from(new Uint8Array(digest), (byte) =>
		byte.toString(16).padStart(2, '0')
	).join('');
	return current.generation === receipt;
}

export function scheduleStaticOAuthCredentialTestExpiry(
	expiresAt: string,
	onExpire: () => void,
	now = Date.now()
): () => void {
	const expiresIn = Math.max(0, Date.parse(expiresAt) - now);
	const timeout = setTimeout(onExpire, expiresIn);
	return () => clearTimeout(timeout);
}
