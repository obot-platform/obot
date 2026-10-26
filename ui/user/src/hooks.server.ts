import type { Handle, HandleServerError } from '@sveltejs/kit/hooks';

export const handle: Handle = async ({ event, resolve }) => {
	return resolve(event, {
		filterSerializedResponseHeaders: (name) => name === 'content-type'
	});
};

export const handleError: HandleServerError = ({ error }) => {
	return {
		status: 500,
		message: error instanceof Error ? error.message : String(error)
	};
};
