<script lang="ts" generics="T extends Record<string, string | number | null | undefined>">
	import { page } from '$app/state';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { UserService } from '$lib/services';
	import { AUDIT_LOG_FILTER_OPTIONS_LIMIT } from '$lib/services/user/constants';
	import { goto } from '$lib/url';
	import AuditFilter, { type FilterInput, type FilterOption } from './FilterField.svelte';
	import { X } from '@lucide/svelte';
	import { untrack } from 'svelte';

	type FilterKey = Extract<
		Exclude<
			keyof T,
			'query' | 'offset' | 'limit' | 'start_time' | 'end_time' | 'sort_by' | 'sort_order'
		>,
		string
	>;

	type FilterOptionsEndpoint = (
		filterId: string,
		filters?: Partial<T>
	) => Promise<{ options: string[] } | undefined>;

	interface Props {
		filters?: Partial<T>;
		isFilterDisabled?: (key: FilterKey) => boolean;
		isFilterClearable?: (key: FilterKey) => boolean;
		// Used to filter server ids when selecting a multi instance server
		filterOptions?: (option: string, filterId?: FilterKey) => boolean;
		onClose: () => void;
		getUserDisplayName: (userId: string, hasConflict?: () => boolean) => string;
		getFilterDisplayLabel?: (key: string) => string;
		getFilterOptionLabel?: (key: string, value: string) => string;
		getDefaultValue?: <K extends FilterKey>(filter: K) => T[K] | undefined;
		endpoint?: FilterOptionsEndpoint;
	}

	let {
		filters: externFilters,
		isFilterDisabled,
		isFilterClearable,
		onClose,
		getUserDisplayName,
		getFilterDisplayLabel,
		getFilterOptionLabel,
		getDefaultValue,
		filterOptions,
		endpoint = UserService.listMcpAuditLogFilterOptions as FilterOptionsEndpoint
	}: Props = $props();

	let filters = $derived({ ...(externFilters ?? {}) } as Partial<T>);

	type FilterOptions = Record<FilterKey, FilterOption[]>;
	let filtersOptions: FilterOptions = $state({} as FilterOptions);

	type FilterInputs = Record<FilterKey, FilterInput>;
	let filterInputs = $derived(
		(Object.keys(filters ?? {}) as FilterKey[]).reduce((acc, filterId) => {
			acc[filterId] = {
				property: filterId,
				label: getFilterDisplayLabel?.(filterId) ?? filterId.replace(/_(\w)/, ' $1'),
				get tooltip() {
					const count = filtersOptions[filterId]?.length ?? 0;
					return count >= AUDIT_LOG_FILTER_OPTIONS_LIMIT
						? `Showing up to ${AUDIT_LOG_FILTER_OPTIONS_LIMIT} results`
						: undefined;
				},
				get selected() {
					return filters?.[filterId] as string | number | null | undefined;
				},
				set selected(v) {
					(filters as Partial<T>)[filterId] = v as T[typeof filterId];
					// Force Component to react
					filters = { ...filters } as Partial<T>;
				},
				get default() {
					return getDefaultValue?.(filterId) as string | number | null | undefined;
				},
				get options() {
					return filtersOptions[filterId];
				},
				get disabled() {
					return isFilterDisabled?.(filterId) ?? false;
				}
			};
			return acc;
		}, {} as FilterInputs)
	);

	const filterInputsAsArray = $derived(Object.values(filterInputs));

	$effect(() => {
		const processLog = async (filterId: FilterKey) => {
			// Exclude the current filterId from the filters sent to the endpoint,
			// so the backend can return all distinct values for this field
			// given the *other* active filters.
			const otherFilters = Object.fromEntries(
				Object.entries(filters ?? {}).filter(([k]) => k !== filterId)
			) as Partial<T>;
			const response = await endpoint(filterId, otherFilters);

			if (['user_id', 'user_ids'].includes(filterId)) {
				return (
					response?.options
						?.filter((d) => filterOptions?.(d, filterId) ?? true)
						?.map((d) => ({
							id: d,
							label: getUserDisplayName(d, () => response.options.some((id) => id === d))
						})) ?? []
				);
			}

			return (
				response?.options
					?.filter((d) => filterOptions?.(d, filterId) ?? true)
					?.map((d) => ({
						id: d,
						label: getFilterOptionLabel?.(filterId, d) ?? d
					})) ?? []
			);
		};

		const filterInputKeys = Object.keys(filterInputs) as FilterKey[];

		filterInputKeys.forEach((id) => {
			processLog(id).then((options) => {
				untrack(() => {
					filtersOptions[id] = options;
				});
			});
		});
	});

	async function handleApplyFilters() {
		const url = new URL(page.url);
		for (const filterInput of filterInputsAsArray) {
			if (filterInput.selected) {
				url.searchParams.set(filterInput.property, filterInput.selected.toString());
			} else {
				if (filterInput.selected === null) {
					// Clear the search param
					url.searchParams.delete(filterInput.property);
				} else {
					// Override default values
					url.searchParams.set(filterInput.property, '');
				}
			}
		}

		await goto(url, { noScroll: true });

		onClose?.();
	}

	function handleClearAllFilters() {
		filterInputsAsArray
			.filter((filter) =>
				isFilterClearable ? isFilterClearable?.(filter.property as FilterKey) : true
			)
			.forEach((filterInput) => {
				filterInput.selected = '';
			});
	}
</script>

<div
	class="dark:border-base-400 text-base-content h-dvh w-screen border-l border-transparent md:w-lg lg:w-xl"
>
	<div class="relative w-full text-center">
		<h4 class="p-4 text-xl font-semibold">Filters</h4>
		<IconButton class="absolute top-1/2 right-4 -translate-y-1/2" onclick={onClose}>
			<X class="size-5" />
		</IconButton>
	</div>
	<div
		class="default-scrollbar-thin flex h-[calc(100%-60px)] w-full flex-col gap-4 overflow-y-auto p-4 pt-0"
	>
		{#each filterInputsAsArray as filterInput, index (filterInput.property)}
			<AuditFilter
				filter={filterInput}
				onSelect={(_, value) => {
					filterInput.selected = value ?? '';
				}}
				onClearAll={() => {
					// This code section is called only when user click clear all
					// single clear value is handled inside the component
					const key = filterInputsAsArray[index].property;
					filterInputs[key as FilterKey].selected = '';
				}}
				onReset={() => {
					filterInput.selected = null;
				}}
			></AuditFilter>
		{/each}
		<div class="mt-auto flex flex-col gap-2">
			<button
				class="btn btn-secondary text-md w-full rounded-lg px-4 py-2"
				onclick={handleClearAllFilters}>Clear All</button
			>
			<button
				class="btn btn-primary text-md w-full rounded-lg px-4 py-2"
				onclick={handleApplyFilters}>Apply Filters</button
			>
		</div>
	</div>
</div>
