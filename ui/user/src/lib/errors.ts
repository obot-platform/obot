import type { Profile } from '$lib/services';
import { error, redirect } from '@sveltejs/kit';

export function isNotFoundError(e: unknown) {
	if (!(e instanceof Error)) {
		return false;
	}

	const { status } = parseErrorContent(e);
	if (status === 404) {
		return true;
	}

	const message = e.message.toLowerCase();
	return message.includes('404') || message.includes('not found');
}

export function handleRouteError(e: unknown, path: string, profile?: Profile) {
	if (!(e instanceof Error)) {
		throw new Error('Unknown error occurred');
	}

	if (e.message?.includes('403') || e.message?.includes('forbidden')) {
		if (profile?.role === 0) {
			throw redirect(303, `/?rd=${path}`);
		}
		throw error(403, e.message);
	}

	if (e.message?.includes('401') || e.message?.includes('unauthorized')) {
		throw redirect(303, `/?rd=${path}`);
	}

	if (isNotFoundError(e)) {
		if (path.includes('/s/')) {
			throw error(404, `The chatbot at ${path} does not exist`);
		}

		throw error(404, e.message);
	}

	throw error(500, e.message);
}

export function parseErrorContent(e: unknown) {
	if (!(e instanceof Error)) {
		return { status: 500, message: 'Unknown error occurred' };
	}

	// Match format i.e. "400 /path/to/resource: message"
	const errorMatch = e.message.match(/^(\d+)(?:\s+\/[^:]+)?:\s+(.*)/);

	const [, statusCode, messageContent] = errorMatch || [];
	const status = parseInt(statusCode);

	return {
		status: Number.isInteger(status) ? status : 500,
		message: messageContent || 'Unknown error occurred'
	};
}
