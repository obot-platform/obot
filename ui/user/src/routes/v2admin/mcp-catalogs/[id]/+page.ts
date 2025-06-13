import type { PageLoad } from './$types';

export const load: PageLoad = async ({ fetch }) => {
	const mockData = {
		id: 1,
		name: 'Common',
		entries: 100
	};

	return {
		mcpCatalog: mockData
	};
};
