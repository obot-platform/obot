import type { ModelProvider } from './services';

export const sortByCreatedDate = <T extends { created: string }>(a: T, b: T) => {
	return new Date(b.created).getTime() - new Date(a.created).getTime();
};

export const sortModelProviders = (modelProviders: ModelProvider[]) =>
	[...modelProviders].sort((a, b) => a.id.localeCompare(b.id));
