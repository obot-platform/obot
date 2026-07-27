import type { Handle, HandleServerError } from '@sveltejs/kit';

export const handle: Handle = async ({ event, resolve }) => {
	return resolve(event, {
		filterSerializedResponseHeaders: (name) => name === 'content-type'
	});
};

export const handleError: HandleServerError = ({ error }) => {
	return {
		message: error instanceof Error ? error.message : String(error)
	};
};
