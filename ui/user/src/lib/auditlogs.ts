import { page } from '$app/state';
import { isSafe } from './utils';

export function buildSearchParamFiltersArray<T>(
	supportedFilters: (keyof T)[],
	defaultSearchParams?: Partial<Record<keyof T, string | string[] | number | undefined | null>>
): [keyof T, string | undefined | null][] {
	return supportedFilters.map((d): [keyof T, string | undefined | null] => {
		const hasSearchParam = page.url.searchParams.has(d.toString());

		const value = page.url.searchParams.get(d.toString());
		const isValueDefined = isSafe(value);

		return [
			d,
			isValueDefined
				? value
				: hasSearchParam
					? // Value is not defined but has a search param then override with empty string
						''
					: // No search params return default value if exist otherwise return undefined
						(defaultSearchParams?.[d]?.toString() ?? null)
		];
	});
}

export function buildPillSearchParamFilters<T>(
	searchParamFiltersArray: [keyof T, string | undefined | null][],
	additionalFilters?: Partial<Record<keyof T, string | string[] | number | undefined | null>>,
	sortPriority?: Set<string>
): Record<keyof T, string | undefined | null> {
	const base = searchParamFiltersArray
		// exclude start_time and end_time from pills filters
		.filter(([key, value]) => !(key === 'start_time' || key === 'end_time') && isSafe(value))
		.reduce(
			(acc, [key, value]) => {
				acc[key] = value as string | number;
				return acc;
			},
			{} as Record<keyof T, string | number>
		) as Record<keyof T, string | number>;

	return (
		Object.entries({ ...base, ...additionalFilters })
			.filter(([, value]) => !!value)
			// Sort to prioritize props filter keys first, then alphabetically
			.sort((a, b) => {
				if (!sortPriority) return a[0].localeCompare(b[0]);

				// If both keys are in propsFiltersKeys, sort alphabetically
				if (sortPriority.has(a[0]) && sortPriority.has(b[0])) {
					return a[0].localeCompare(b[0]);
				}

				// If only a is in propsFiltersKeys, it comes first
				if (sortPriority.has(a[0])) {
					return -1;
				}

				// If only b is in propsFiltersKeys, it comes first
				if (sortPriority.has(b[0])) {
					return 1;
				}

				// If neither are in propsFiltersKeys, sort alphabetically
				return a[0].localeCompare(b[0]);
			})
			.reduce(
				(acc, val) => {
					acc[val[0]] = val[1] as string;
					return acc;
				},
				{} as Record<string, string | number>
			) as Record<keyof T, string>
	);
}
