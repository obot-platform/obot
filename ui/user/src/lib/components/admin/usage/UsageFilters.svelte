<script module lang="ts">
	export type FilterKey = Exclude<
		keyof AuditLogURLFilters,
		'query' | 'offset' | 'limit' | 'start_time' | 'end_time'
	>;

	export type FilterInput = {
		label: string;
		property: FilterKey;
		selected?: string | number | null;
		default?: string | number | null;
		options: { id: string; label: string }[];
		readonly disabled: boolean;
	};

	export type FilterOption = {
		label: string;
		id: string;
	};
</script>

<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import type { AuditLogURLFilters } from '$lib/services/admin/types';
	import { X } from 'lucide-svelte';
	import AuditFilter from '../audit-logs/AuditFilter.svelte';
	import { AdminService } from '$lib/services';
	import { untrack } from 'svelte';

	interface Props {
		filters?: AuditLogURLFilters;
		isFilterDisabled?: (key: keyof AuditLogURLFilters) => boolean;
		// Used to filter server ids when selecting a multi instance server
		filterOptions?: (option: string, filterId?: keyof AuditLogURLFilters) => boolean;
		onClose: () => void;
		getUserDisplayName: (userId: string) => string;
		getFilterDisplayLabel?: (key: keyof AuditLogURLFilters) => string;
		getDefaultValue?: <T extends keyof AuditLogURLFilters>(filter: T) => AuditLogURLFilters[T];
	}

	let {
		filters: externFilters,
		isFilterDisabled,
		getUserDisplayName,
		getFilterDisplayLabel,
		getDefaultValue,
		filterOptions,
		onClose
	}: Props = $props();

	const url = new URL(page.url);

	let filters = $derived({ ...(externFilters ?? {}) });

	const filterKeys: FilterKey[] = $state(['user_id', 'mcp_server_display_name']);

	type FilterOptions = Record<FilterKey, FilterOption[]>;
	let filtersOptions: FilterOptions = $state({} as FilterOptions);

	type FilterInputs = Record<FilterKey, FilterInput>;
	let filterInputs = $derived(
		filterKeys.reduce((acc, filterId) => {
			acc[filterId] = {
				property: filterId,
				label: getFilterDisplayLabel?.(filterId) ?? filterId.replace(/_(\w)/, ' $1'),
				get selected() {
					return filters?.[filterId];
				},
				set selected(v) {
					filters[filterId] = v;
					// Force Component to react
					filters = { ...filters };
				},
				get default() {
					return getDefaultValue?.(filterId);
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
		const processLog = async (filterId: keyof AuditLogURLFilters) => {
			const response = await AdminService.listAuditLogFilterOptions(filterId);

			if (filterId === 'user_id') {
				return (
					response?.options
						?.filter((d) => filterOptions?.(d, filterId) ?? true)
						?.map((d) => ({
							id: d,
							label: getUserDisplayName?.(d) ?? d
						})) ?? []
				);
			}

			return (
				response?.options
					?.filter((d) => filterOptions?.(d, filterId) ?? true)
					?.map((d) => ({
						id: d,
						label: d
					})) ?? []
			);
		};

		filterKeys.forEach((id) => {
			processLog(id).then((options) => {
				untrack(() => {
					filtersOptions[id] = options;
				});
			});
		});
	});

	async function handleApplyFilters() {
		for (const filterInput of filterInputsAsArray) {
			if (filterInput.selected) {
				url.searchParams.set(
					filterInput.property,
					encodeURIComponent(filterInput.selected.toString())
				);
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
		filterInputsAsArray.forEach((filterInput) => {
			filterInput.selected = '';
		});
	}
</script>

<div class="dark:border-surface3 h-dvh w-screen border-l border-transparent md:w-sm">
	<div class="relative w-full text-center">
		<h4 class="p-4 text-xl font-semibold">Filters</h4>
		<button class="icon-button absolute top-1/2 right-4 -translate-y-1/2" onclick={onClose}>
			<X class="size-5" />
		</button>
	</div>
	<div
		class="default-scrollbar-thin flex h-[calc(100%-60px)] flex-col gap-4 overflow-y-auto p-4 pt-0"
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
					filterInputs[key].selected = '';
				}}
				onReset={() => {
					filterInput.selected = null;
				}}
			/>
		{/each}
		<div class="mt-auto flex flex-col gap-2">
			<button
				class="button-secondary text-md w-full rounded-lg px-4 py-2"
				onclick={handleClearAllFilters}>Clear All</button
			>
			<button
				class="button-primary text-md w-full rounded-lg px-4 py-2"
				onclick={handleApplyFilters}>Apply Filters</button
			>
		</div>
	</div>
</div>
