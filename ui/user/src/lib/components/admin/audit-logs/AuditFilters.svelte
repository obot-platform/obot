<script module lang="ts">
	export type FilterKey = Exclude<
		keyof AuditLogURLFilters,
		'query' | 'offset' | 'limit' | 'start_time' | 'end_time'
	>;

	export type FilterInput = {
		label: string;
		property: FilterKey;
		selected: string | number;
		options: { id: string; label: string }[];
	};

	export type FilterOption = {
		label: string;
		id: string;
	};
</script>

<script lang="ts">
	import AuditFilter from './AuditFilter.svelte';
	import { X } from 'lucide-svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import type { AuditLogURLFilters, OrgUser } from '$lib/services/admin/types';
	import { AdminService } from '$lib/services';
	import { untrack } from 'svelte';

	interface Props {
		filters?: AuditLogURLFilters;
		onClose: () => void;
		fetchUserById: (userId: string) => Promise<OrgUser | undefined>;
		getFilterDisplayLabel?: (key: keyof AuditLogURLFilters) => string;
	}

	let { filters: externFilters, onClose, fetchUserById, getFilterDisplayLabel }: Props = $props();

	let filters = $derived({ ...(externFilters ?? {}) });

	type FilterOptions = Record<FilterKey, FilterOption[]>;
	let filtersOptions: FilterOptions = $state({} as FilterOptions);

	type FilterInputs = Record<FilterKey, FilterInput>;
	let filterInputs = (
		[
			'user_id',
			'mcp_id',
			'mcp_server_display_name',
			'mcp_server_catalog_entry_name',
			'call_type',
			'client_name',
			'client_version',
			'client_ip',
			'response_status',
			'session_id'
		] as FilterKey[]
	).reduce((acc, filterId) => {
		acc[filterId] = {
			property: filterId,
			label: getFilterDisplayLabel?.(filterId) ?? filterId.replace(/_(\w)/, ' $1'),
			get selected() {
				return filters?.[filterId] ?? '';
			},
			set selected(v) {
				filters[filterId] = v ?? '';
				// Force Component to react
				filters = { ...filters };
			},
			get options() {
				return filtersOptions[filterId];
			}
		};
		return acc;
	}, {} as FilterInputs);

	const filterInputsAsArray = $derived(Object.values(filterInputs));

	$effect(() => {
		const processLog = async (filterId: string) => {
			const response = await AdminService.listAuditLogFilterOptions(filterId);

			if (filterId === 'user_id') {
				return await Promise.all(
					response.options
						.map((d) => fetchUserById(d).then((user) => ({ id: d, label: user?.displayName ?? d })))
						.filter(Boolean)
				);
			}

			return response.options.map((d) => ({
				id: d,
				label: d
			}));
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

	function handleApplyFilters() {
		const url = page.url;

		for (const filterInput of filterInputsAsArray) {
			if (filterInput.selected) {
				url.searchParams.set(
					filterInput.property,
					encodeURIComponent(filterInput.selected.toString())
				);
			} else {
				page.url.searchParams.delete(filterInput.property);
			}
		}

		goto(url, { noScroll: true });
	}

	function handleClearAllFilters() {
		filterInputsAsArray.forEach((filterInput) => {
			filterInput.selected = '';
		});
	}
</script>

<div class="dark:border-surface3 h-full w-screen border-l border-transparent md:w-lg lg:w-xl">
	<div class="relative w-full text-center">
		<h4 class="p-4 text-xl font-semibold">Filters</h4>
		<button class="icon-button absolute top-1/2 right-4 -translate-y-1/2" onclick={onClose}>
			<X class="size-5" />
		</button>
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
					filterInputs[key].selected = '';
				}}
			></AuditFilter>
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
