import type { Profile } from '$lib/services';
import { error, redirect } from '@sveltejs/kit';

const defaultErrorMessage = 'Unknown error occurred';

export class HttpError extends Error {
	constructor(
		public statusCode: number,
		message: string
	) {
		super(message);
		this.name = 'HttpError';
	}
}

export function createHttpError(statusCode: number, path: string, body: string): HttpError {
	return new HttpError(statusCode, `${statusCode} ${path}: ${body}`);
}

export function getHttpStatusCode(e: unknown): number | undefined {
	if (e instanceof HttpError) {
		return e.statusCode;
	}
	return undefined;
}

function toAppError(e: Error): App.Error {
	return {
		message: e.message || defaultErrorMessage
	};
}

function parseHttpErrorMessage(message: string): string {
	// Match format i.e. "400 /path/to/resource: message"
	const errorMatch = message.match(/^\d+\s+\/[^:]+:\s+(.*)/s);
	return parseResponseError(errorMatch?.[1] || message || defaultErrorMessage);
}

function parseResponseError(message: string): string {
	try {
		const body = JSON.parse(message) as unknown;
		if (
			typeof body === 'object' &&
			body !== null &&
			'error' in body &&
			typeof body.error === 'string'
		) {
			return body.error;
		}
	} catch {
		// The response wasn't JSON, so use it as-is.
	}

	return message;
}

export function handleRouteError(e: unknown, path: string, profile?: Profile): never {
	if (!(e instanceof Error)) {
		throw error(500, { message: 'Unknown error occurred' });
	}

	const appError = toAppError(e);
	const statusCode = getHttpStatusCode(e) ?? 500;

	if (statusCode === 403) {
		if (profile?.role === 0) {
			throw redirect(303, `/?rd=${path}`);
		}
		throw error(403, appError);
	}

	if (statusCode === 401) {
		throw redirect(303, `/?rd=${path}`);
	}

	if (statusCode === 404) {
		if (path.includes('/s/')) {
			throw error(404, `The chatbot at ${path} does not exist`);
		}

		throw error(404, appError);
	}

	throw error(statusCode, appError);
}

export function parseErrorContent(e: unknown) {
	if (!(e instanceof Error)) {
		return { status: 500, message: 'Unknown error occurred' };
	}

	const statusCode = getHttpStatusCode(e);
	if (statusCode !== undefined) {
		return {
			status: statusCode,
			message: parseHttpErrorMessage(e.message)
		};
	}

	// Match format i.e. "400 /path/to/resource: message"
	const errorMatch = e.message.match(/^(\d+)(?:\s+\/[^:]+)?:\s+(.*)/);
	if (!errorMatch) {
		return { status: 500, message: parseResponseError(e.message || defaultErrorMessage) };
	}

	const [, legacyStatusCode, messageContent] = errorMatch;
	const status = parseInt(legacyStatusCode);

	return {
		status: Number.isInteger(status) ? status : 500,
		message: parseResponseError(messageContent || defaultErrorMessage)
	};
}

export function isAbortError(err: unknown) {
	return (
		(err instanceof Error ||
			(typeof DOMException !== 'undefined' && err instanceof DOMException)) &&
		err.name === 'AbortError'
	);
}
