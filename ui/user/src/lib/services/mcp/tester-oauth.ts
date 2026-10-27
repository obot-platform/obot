import { replaceState } from '$app/navigation';
import {
	auth,
	extractWWWAuthenticateParams,
	UnauthorizedError,
	type AuthResult,
	type OAuthDiscoveryState,
	type OAuthClientProvider
} from '@modelcontextprotocol/sdk/client/auth.js';
import type {
	OAuthClientInformationMixed,
	OAuthClientMetadata
} from '@modelcontextprotocol/sdk/shared/auth.js';

interface PendingAuthorization {
	state: string;
	verifier?: string;
	client?: OAuthClientInformationMixed;
	redirectUrl: string;
	discovery?: OAuthDiscoveryState;
}

// The tester already authenticates to Obot with its session cookie. OAuth here
// completes the captured upstream requirements; downstream tokens are not retained.
export class TesterOAuth implements OAuthClientProvider {
	readonly #key: string;
	#pending?: PendingAuthorization;
	#authorizing?: Promise<AuthResult>;

	constructor(
		readonly serverUrl: URL,
		readonly onAuthorization: (url?: string) => void
	) {
		this.#key = `mcp-tester-oauth:${serverUrl.href}`;
		// Ordinary tester sessions stay memory-only; restore state only on an OAuth return.
		if (
			typeof window !== 'undefined' &&
			new URL(window.location.href).searchParams.get('mcp_tester_oauth') === serverUrl.pathname
		) {
			const saved = sessionStorage.getItem(this.#key);
			if (saved) this.#pending = JSON.parse(saved) as PendingAuthorization;
		}
	}

	get redirectUrl(): string {
		if (!this.#pending) {
			const url = new URL(window.location.href);
			url.searchParams.set('mcp_tester_oauth', this.serverUrl.pathname);
			this.#pending = { state: crypto.randomUUID(), redirectUrl: url.href };
			this.#save();
		}

		return this.#pending.redirectUrl;
	}

	get clientMetadata(): OAuthClientMetadata {
		return {
			client_name: 'Obot MCP Tester',
			redirect_uris: [this.redirectUrl],
			grant_types: ['authorization_code'],
			response_types: ['code'],
			token_endpoint_auth_method: 'none'
		};
	}

	state(): string {
		void this.redirectUrl;
		return this.#pending!.state;
	}

	clientInformation() {
		return this.#pending?.client;
	}

	saveClientInformation(client: OAuthClientInformationMixed) {
		void this.redirectUrl;
		this.#pending!.client = client;
		this.#save();
	}

	discoveryState() {
		return this.#pending?.discovery;
	}

	saveDiscoveryState(discovery: OAuthDiscoveryState) {
		void this.redirectUrl;
		this.#pending!.discovery = discovery;
		this.#save();
	}

	tokens() {
		return undefined;
	}
	saveTokens() {
		/* The Obot session cookie remains the tester's credential. */
	}

	saveCodeVerifier(verifier: string) {
		void this.redirectUrl;
		this.#pending!.verifier = verifier;
		this.#save();
	}

	codeVerifier(): string {
		if (!this.#pending?.verifier) throw new Error('Missing tester OAuth verifier');
		return this.#pending.verifier;
	}

	redirectToAuthorization(url: URL) {
		this.onAuthorization(url.href);
	}

	#save() {
		sessionStorage.setItem(this.#key, JSON.stringify(this.#pending));
	}

	async complete(fetcher: typeof fetch): Promise<void> {
		// OAuth callbacks only exist in the browser; non-browser sessions have nothing to restore.
		if (typeof window === 'undefined') return;

		const url = new URL(window.location.href);
		if (url.searchParams.get('mcp_tester_oauth') !== this.serverUrl.pathname) return;
		if (!this.#pending || url.searchParams.get('state') !== this.#pending.state) {
			throw new Error('Invalid tester OAuth state');
		}

		try {
			const error = url.searchParams.get('error');
			if (error) throw new Error(url.searchParams.get('error_description') || error);
			const code = url.searchParams.get('code');
			if (!code) throw new Error('Missing tester OAuth authorization code');

			await auth(this, { serverUrl: this.serverUrl, authorizationCode: code, fetchFn: fetcher });
		} finally {
			const returnUrl = new URL(this.#pending.redirectUrl);
			returnUrl.searchParams.delete('mcp_tester_oauth');
			// Restore the exact pre-OAuth location, including its inspector selection.
			replaceState(returnUrl, {});
			sessionStorage.removeItem(this.#key);
			this.#pending = undefined;
		}
	}

	fetch(fetcher: typeof fetch): typeof fetch {
		return async (input, init) => {
			const response = await fetcher(input, init);
			if (response.status !== 401 && response.status !== 403) return response;
			const { error, scope, resourceMetadataUrl } = extractWWWAuthenticateParams(response);
			if (error !== 'insufficient_scope' || !scope) {
				// A vMCP 401 without a scope ticket still requires Obot authentication.
				if (response.status === 401) this.onAuthorization();

				return response;
			}

			// Use the captured scope, never another discovery probe or a replayed tool call.
			// Parallel discovery requests share one PKCE flow instead of overwriting its verifier.
			this.#authorizing ??= auth(this, {
				serverUrl: this.serverUrl,
				scope,
				resourceMetadataUrl,
				fetchFn: fetcher
			}).finally(() => {
				this.#authorizing = undefined;
			});
			await this.#authorizing;
			throw new UnauthorizedError();
		};
	}
}
