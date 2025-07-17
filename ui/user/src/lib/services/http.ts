import { UNAUTHORIZED_PATHS } from '$lib/constants';
import { profile } from '$lib/stores';
import errors from '$lib/stores/errors.svelte';

export let baseURL = 'http://localhost:8080/api';

if (typeof window !== 'undefined') {
	baseURL = baseURL.replace('http://localhost:8080', window.location.origin);
}

interface GetOptions {
	blob?: boolean;
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

export async function doGet(path: string, opts?: GetOptions): Promise<unknown> {
	const f = opts?.fetch || fetch;
	const resp = await f(baseURL + path, {
		headers: {
			// Pass the browser timezone as a request header.
			// This is consumed during authentication to set the user's default timezone in Obot.
			// The timezone is plumbed down to tools at runtime as an environment variable.
			'x-obot-user-timezone': Intl.DateTimeFormat().resolvedOptions().timeZone
		},
		signal: opts?.signal
	});

	if (!resp.ok) {
		if (resp.status === 401) {
			handle401Redirect();
		}
		const body = await resp.text();
		const e = new Error(`${resp.status} ${path}: ${body}`);
		if (opts?.dontLogErrors) {
			throw e;
		}
		errors.items.push(e);
		throw e;
	}

	if (opts?.blob) {
		return await resp.blob();
	}

	return await resp.json();
}

export async function doDelete(path: string, opts?: { signal?: AbortSignal }): Promise<unknown> {
	const resp = await fetch(baseURL + path, {
		method: 'DELETE',
		signal: opts?.signal
	});

	if (!resp.ok && resp.status === 401) {
		handle401Redirect();
	}
	return handleResponse(resp, path);
}

export async function doPut(
	path: string,
	input?: string | object | Blob,
	opts?: {
		dontLogErrors?: boolean;
		fetch?: typeof fetch;
		signal?: AbortSignal;
	}
): Promise<unknown> {
	return await doWithBody('PUT', path, input, opts);
}

async function handleResponse(
	resp: Response,
	path: string,
	opts?: {
		dontLogErrors?: boolean;
	}
): Promise<unknown> {
	if (!resp.ok) {
		const body = await resp.text();
		const e = new Error(`${resp.status} ${path}: ${body}`);
		if (opts?.dontLogErrors) {
			throw e;
		}
		errors.items.push(e);
		throw e;
	}
	if (resp.headers.get('Content-Type') === 'application/json') {
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
		fetch?: typeof fetch;
		headers?: Record<string, string>;
		signal?: AbortSignal;
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
			headers: { ...headers, ...opts?.headers },
			body,
			signal: opts?.signal
		});

		if (!resp.ok && resp.status === 401) {
			handle401Redirect();
		}
		return handleResponse(resp, path, opts);
	} catch (e) {
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
