import type { HandleClientError } from '@sveltejs/kit';

export const handleError: HandleClientError = ({ error }) => {
	return {
		message: error instanceof Error ? error.message : String(error)
	};
};
