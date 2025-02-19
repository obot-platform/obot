import errors from '$lib/stores/errors.svelte';

export let baseURL = 'http://localhost:8080/api';

if (typeof window !== 'undefined') {
	baseURL = baseURL.replace('http://localhost:8080', window.location.origin);
}

interface GetOptions {
	blob?: boolean;
	dontLogErrors?: boolean;
}

export async function doGet(path: string, opts?: GetOptions): Promise<unknown> {
	const resp = await fetch(baseURL + path, {
		headers: {
			// Pass the browser timezone as a request header.
			// This is consumed during authentication to set the user's default timezone in Obot.
			// The timezone is plumbed down to tools at runtime as an environment variable.
			'x-obot-user-timezone': Intl.DateTimeFormat().resolvedOptions().timeZone
		}
	});

	if (!resp.ok) {
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

export async function doDelete(path: string): Promise<unknown> {
	const resp = await fetch(baseURL + path, {
		method: 'DELETE'
	});
	return handleResponse(resp, path);
}

export async function doPut(path: string, input?: string | object | Blob): Promise<unknown> {
	return await doWithBody('PUT', path, input);
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
	input?: string | object | Blob,
	opts?: {
		dontLogErrors?: boolean;
	}
): Promise<unknown> {
	let headers: Record<string, string> | undefined;
	if (input instanceof Blob) {
		headers = { 'Content-Type': 'application/octet-stream' };
	} else if (typeof input === 'object') {
		input = JSON.stringify(input);
		headers = { 'Content-Type': 'application/json' };
	} else if (input) {
		headers = { 'Content-Type': 'text/plain' };
	}
	try {
		const resp = await fetch(baseURL + path, {
			method: method,
			headers: headers,
			body: input
		});
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
	}
): Promise<unknown> {
	return await doWithBody('POST', path, input, opts);
}
