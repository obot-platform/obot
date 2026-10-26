import type { HandleClientError } from '@sveltejs/kit/hooks';

export const handleError: HandleClientError = ({ error }) => {
	return {
		status: 500,
		message: error instanceof Error ? error.message : String(error)
	};
};
