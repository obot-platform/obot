import {
	ENVIRONMENT_RESTRICTED_COOKIE,
	ENVIRONMENT_RESTRICTED_LOGIN_PATH,
	UNAUTHORIZED_PATHS
} from '$lib/constants';
import { hasCookie } from '$lib/cookies';
import { createHttpError } from '$lib/errors';
import { getLocale } from '$lib/i18n';
import { profile } from '$lib/stores';
import errors from '$lib/stores/errors.svelte';

// For SSR, use VITE_API_TARGET if set (for remote API development)
// For browser, use window.location.origin (requests go through Vite proxy)
const apiTarget = import.meta.env.VITE_API_TARGET as string | undefined;
export let baseURL = 'http://localhost:8080/api';

if (typeof window !== 'undefined') {
	baseURL = baseURL.replace('http://localhost:8080', window.location.origin);
} else if (apiTarget) {
	// SSR: use the configured API target directly
	baseURL = apiTarget.endsWith('/api') ? apiTarget : apiTarget + '/api';
}

// API token for development - set VITE_API_TOKEN to bypass OAuth
// Use `OBOT_BASE_URL=https://<hostname>/api obot token` to get a token
const apiToken = import.meta.env.VITE_API_TOKEN as string | undefined;

function getAuthHeaders(): Record<string, string> {
	if (apiToken) {
		return { Authorization: `Bearer ${apiToken}` };
	}
	return {};
}

function getRequestHeaders(extra?: Record<string, string>): Record<string, string> {
	return {
		...getAuthHeaders(),
		'Accept-Language': getLocale() || 'en',
		...extra
	};
}

interface GetOptions {
	blob?: boolean;
	/** When true, return response body as plain text (e.g. for text/markdown). */
	text?: boolean;
	fetch?: typeof fetch;
	dontLogErrors?: boolean;
	signal?: AbortSignal;
}

function handle401Redirect() {
	if (typeof window === 'undefined') return;
	const currentPath = window.location.pathname;

	// User was logged in, but the session expired
	// Set expired and re-login dialog will show
	if (profile.current.loaded === true) {
		profile.current.expired = true;
		return;
	}

	// Not logged in, so if the user is
	// not already on an unauthorized page, redirect to it
	if (!UNAUTHORIZED_PATHS.has(currentPath)) {
		window.location.href = `/?rd=${encodeURIComponent(currentPath)}`;
	}
}

// handleRestrictedSignOut sends an open page to the login page once the server has signed its user out of a
// restricted environment, so the login page can say why. Other 402s, like a plan's limit, leave the page alone.
function handleRestrictedSignOut() {
	if (typeof window === 'undefined') return;
	if (UNAUTHORIZED_PATHS.has(window.location.pathname)) return;
	if (hasCookie(ENVIRONMENT_RESTRICTED_COOKIE)) {
		window.location.href = ENVIRONMENT_RESTRICTED_LOGIN_PATH;
	}
}

// handleSignedOut follows the server when it ends the session.
function handleSignedOut(status: number) {
	if (status === 401) {
		handle401Redirect();
	} else if (status === 402) {
		handleRestrictedSignOut();
	}
}

export async function doGet(path: string, opts?: GetOptions): Promise<unknown> {
	const resp = await doGetForResponse(path, opts);

	if (opts?.blob) {
		return await resp.blob();
	}

	if (opts?.text) {
		return await resp.text();
	}

	return await resp.json();
}

// Returns a successful raw GET response so callers can consume both its body
// and response headers, as required for file downloads.
export async function doGetForResponse(path: string, opts?: GetOptions): Promise<Response> {
	const f = opts?.fetch || fetch;
	const resp = await f(baseURL + path, {
		headers: getRequestHeaders({
			// Pass the browser timezone as a request header.
			// This is consumed during authentication to set the user's default timezone in Obot.
			// The timezone is plumbed down to tools at runtime as an environment variable.
			'x-obot-user-timezone': Intl.DateTimeFormat().resolvedOptions().timeZone
		}),
		signal: opts?.signal
	});

	if (!resp.ok) {
		handleSignedOut(resp.status);
		const body = await resp.text();
		const e = createHttpError(resp.status, path, body);
		if (opts?.dontLogErrors) {
			throw e;
		}
		errors.items.push(e);
		throw e;
	}

	return resp;
}

type ResponseHandler = (
	resp: Response,
	path: string,
	opts?: { dontLogErrors?: boolean }
) => Promise<unknown>;

export type ErrorHandler = (error: unknown) => void;

export async function doDelete(
	path: string,
	opts?: {
		signal?: AbortSignal;
		dontLogErrors?: boolean;
		fetch?: typeof fetch;
		responseHandler?: ResponseHandler;
		keepalive?: boolean;
	}
): Promise<unknown> {
	const f = opts?.fetch || fetch;
	const resp = await f(baseURL + path, {
		method: 'DELETE',
		headers: getRequestHeaders(),
		...(opts?.keepalive ? { keepalive: true } : { signal: opts?.signal })
	});

	if (!resp.ok) {
		handleSignedOut(resp.status);
	}
	return opts?.responseHandler?.(resp, path, opts) ?? handleResponse(resp, path, opts);
}

export async function doPut(
	path: string,
	input?: string | object | Blob,
	opts?: {
		dontLogErrors?: boolean;
		errorHandler?: ErrorHandler;
		fetch?: typeof fetch;
		signal?: AbortSignal;
	}
): Promise<unknown> {
	return await doWithBody('PUT', path, input, opts);
}

export async function handleResponse(
	resp: Response,
	path: string,
	opts?: {
		dontLogErrors?: boolean;
		errorHandler?: ErrorHandler;
	}
): Promise<unknown> {
	if (!resp.ok) {
		const body = await resp.text();
		const e = createHttpError(resp.status, path, body);
		if (opts?.dontLogErrors || opts?.errorHandler) {
			throw e;
		}
		errors.items.push(e);
		throw e;
	}
	if (resp.headers.get('Content-Type')?.includes('application/json')) {
		return resp.json();
	}
	return resp.text();
}

export async function doWithBody(
	method: string,
	path: string,
	input?: string | object | Blob | FormData,
	opts?: {
		dontLogErrors?: boolean;
		errorHandler?: ErrorHandler;
		fetch?: typeof fetch;
		headers?: Record<string, string>;
		signal?: AbortSignal;
		responseHandler?: ResponseHandler;
	}
): Promise<unknown> {
	let headers: Record<string, string> | undefined;
	let body: BodyInit | undefined;

	if (input instanceof FormData) {
		// Let the browser automatically set the Content-Type with proper boundary.
		body = input;
		headers = undefined;
	} else if (input instanceof Blob) {
		body = input;
		headers = { 'Content-Type': 'application/octet-stream' };
	} else if (typeof input === 'object' && input !== null) {
		body = JSON.stringify(input);
		headers = { 'Content-Type': 'application/json' };
	} else if (typeof input === 'string') {
		body = input;
		headers = { 'Content-Type': 'text/plain' };
	}

	try {
		const f = opts?.fetch || fetch;
		const resp = await f(baseURL + path, {
			method,
			headers: getRequestHeaders({ ...headers, ...opts?.headers }),
			body,
			signal: opts?.signal
		});

		if (!resp.ok) {
			handleSignedOut(resp.status);
		}
		return opts?.responseHandler?.(resp, path, opts) ?? handleResponse(resp, path, opts);
	} catch (e) {
		if (opts?.errorHandler) {
			opts.errorHandler(e);
			throw e;
		}
		if (opts?.dontLogErrors) {
			throw e;
		}
		errors.append(e);
		throw e;
	}
}

export async function doPost(
	path: string,
	input: string | object | Blob,
	opts?: {
		dontLogErrors?: boolean;
		errorHandler?: ErrorHandler;
		fetch?: typeof fetch;
		headers?: Record<string, string>;
		signal?: AbortSignal;
	}
): Promise<unknown> {
	return await doWithBody('POST', path, input, opts);
}

export async function doPatch(
	path: string,
	input?: string | object | Blob,
	opts?: {
		dontLogErrors?: boolean;
		fetch?: typeof fetch;
		signal?: AbortSignal;
	}
): Promise<unknown> {
	return await doWithBody('PATCH', path, input, opts);
}

export type Fetcher = typeof fetch;

export type PaginatedResponse<T> = {
	items: T[] | null;
	total: number;
	offset: number;
	limit: number;
};
