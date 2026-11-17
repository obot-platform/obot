import { m } from '$lib/i18n';
import type { Profile } from '$lib/services';
import type { AgentReadinessReason } from '$lib/services/agent-identities/types';
import { error, redirect } from '@sveltejs/kit';

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
		status: e instanceof HttpError ? e.statusCode : 500,
		message: e.message || m.core_unknown_error_occurred()
	};
}

// httpErrorBody returns the response body that an HttpError's message carries after its status and
// path, as in "400 /path/to/resource: message".
function httpErrorBody(message: string): string | undefined {
	return message.match(/^\d+\s+\/[^:]+:\s+(.*)/s)?.[1];
}

// httpErrorReason returns why a request failed, as the server said it, or nothing when it wasn't an HTTP error or the
// server didn't say.
export function httpErrorReason(e: unknown): string | undefined {
	if (!(e instanceof HttpError)) {
		return undefined;
	}
	const body = httpErrorBody(e.message)?.trim();
	return body ? parseResponseError(body) : undefined;
}

function parseHttpErrorMessage(message: string): string {
	return parseResponseError(httpErrorBody(message) || message || m.core_unknown_error_occurred());
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
		throw error(500, m.core_unknown_error_occurred());
	}

	const appError = toAppError(e);
	const statusCode = getHttpStatusCode(e) ?? 500;

	if (statusCode === 403) {
		if (profile?.role === 0) {
			throw redirect(303, `/?rd=${encodeURIComponent(path)}`);
		}
		throw error(403, appError.message);
	}

	if (statusCode === 401) {
		throw redirect(303, `/?rd=${encodeURIComponent(path)}`);
	}

	if (statusCode === 404) {
		if (path.includes('/s/')) {
			throw error(404, m.core_chatbot_not_found({ path }));
		}

		throw error(404, appError.message);
	}

	throw error(statusCode, appError.message);
}

export function parseErrorContent(e: unknown) {
	if (!(e instanceof Error)) {
		return { status: 500, message: m.core_unknown_error_occurred() };
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
		return {
			status: 500,
			message: parseResponseError(e.message || m.core_unknown_error_occurred())
		};
	}

	const [, legacyStatusCode, messageContent] = errorMatch;
	const status = parseInt(legacyStatusCode);

	return {
		status: Number.isInteger(status) ? status : 500,
		message: parseResponseError(messageContent || m.core_unknown_error_occurred())
	};
}

// A reason an error response gives for the failure (the body's "reasons", as the vMCP readiness API reports
// them), such as the configuration key that is still missing. It never carries a value.
export type ErrorReason = Pick<AgentReadinessReason, 'componentID' | 'field' | 'profileID'> & {
	code: string;
};

// The structured reasons of an HTTP error, or none when its body is plain text or has no reasons.
export function parseErrorReasons(e: unknown): ErrorReason[] {
	if (!(e instanceof HttpError)) return [];
	const body = httpErrorBody(e.message) ?? e.message;
	try {
		const parsed = JSON.parse(body) as { reasons?: unknown };
		if (!Array.isArray(parsed?.reasons)) return [];
		return parsed.reasons.filter(
			(reason): reason is ErrorReason =>
				typeof reason === 'object' && reason !== null && typeof reason.code === 'string'
		);
	} catch {
		return [];
	}
}

export function isAbortError(err: unknown) {
	return (
		(err instanceof Error ||
			(typeof DOMException !== 'undefined' && err instanceof DOMException)) &&
		err.name === 'AbortError'
	);
}
