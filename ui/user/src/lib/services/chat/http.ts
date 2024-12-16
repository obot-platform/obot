// import from stores/errors and not stores to avoid a circular dependency
import errors from '$lib/stores/errors';

export let baseURL = 'http://localhost:8080/api';

if (typeof window !== 'undefined') {
	baseURL = baseURL.replace('http://localhost:8080', window.location.origin);
}

interface GetOptions {
	blob?: boolean;
}

export async function doGet(path: string, opts?: GetOptions): Promise<unknown> {
	const resp = await fetch(baseURL + path);
	if (!resp.ok) {
		const body = await resp.text();
		const e = new Error(`${resp.status} ${path}: ${body}`);
		errors.append(e);
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
	let headers: Record<string, string> | undefined;
	if (input instanceof Blob) {
		headers = {
			'Content-Type': 'application/octet-stream'
		};
	} else if (typeof input === 'object') {
		input = JSON.stringify(input);
		headers = {
			'Content-Type': 'application/json'
		};
	} else if (input) {
		headers = {
			'Content-Type': 'text/plain'
		};
	}
	const resp = await fetch(baseURL + path, {
		method: 'PUT',
		headers: headers,
		body: input
	});
	return handleResponse(resp, path);
}

async function handleResponse(resp: Response, path: string): Promise<unknown> {
	if (!resp.ok) {
		const body = await resp.text();
		const e = new Error(`${resp.status} ${path}: ${body}`);
		errors.append(e);
		throw e;
	}
	if (resp.headers.get('Content-Type') === 'application/json') {
		return resp.json();
	}
	return resp.text();
}

export async function doPost(path: string, input: string | object | Blob): Promise<unknown> {
	let contentType = 'text/plain';
	if (input instanceof Blob) {
		contentType = 'application/octet-stream';
	} else if (typeof input === 'object') {
		input = JSON.stringify(input);
		contentType = 'application/json';
	}
	const resp = await fetch(baseURL + path, {
		method: 'POST',
		headers: {
			'Content-Type': contentType
		},
		body: input
	});
	return handleResponse(resp, path);
}
