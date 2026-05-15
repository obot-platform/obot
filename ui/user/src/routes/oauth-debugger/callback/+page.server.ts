import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ setHeaders }) => {
	setHeaders({
		'cache-control': 'no-store, max-age=0',
		pragma: 'no-cache'
	});
	return {};
};
