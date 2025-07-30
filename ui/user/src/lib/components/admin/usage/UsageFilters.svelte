<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import Select from '$lib/components/Select.svelte';
	import type { UsageStatsFilters, AuditLogUsageStats, OrgUser } from '$lib/services/admin/types';
	import { X } from 'lucide-svelte';

	interface Props {
		usageStats?: AuditLogUsageStats;
		users: OrgUser[];
		onClose: () => void;
		filters?: UsageStatsFilters;
	}

	type FilterSet = {
		label: string;
		property: string;
		values: Record<string, FilterValue>;
		selected: string;
	};

	type FilterValue = {
		label: string;
		id: string;
	};

	function generateFilters(
		stats?: AuditLogUsageStats,
		users: OrgUser[] = [],
		filters?: UsageStatsFilters
	) {
		const filterSets: FilterSet[] = [
			{
				label: 'User',
				property: 'userId',
				values: {},
				selected: filters?.userId ?? ''
			},
			{
				label: 'MCP Server',
				property: 'mcpServerDisplayName',
				values: {},
				selected: filters?.mcpServerDisplayName ?? ''
			}
		];

		// Collect all unique users and servers from the usage stats
		const userIds = new Set<string>();
		const serverNames = new Set<string>();

		if (stats?.items) {
			for (const item of stats.items) {
				if (item.mcpServerDisplayName) {
					serverNames.add(item.mcpServerDisplayName);
				}

				// Extract user IDs from tool calls
				for (const toolCall of item.toolCalls ?? []) {
					for (const callItem of toolCall.items ?? []) {
						if (callItem.userID) {
							userIds.add(callItem.userID);
						}
					}
				}
			}
		}

		// Populate user filter values
		for (const userId of userIds) {
			const user = users.find((u) => u.id === userId);
			filterSets[0].values[userId] = {
				label: user?.email ?? 'Unknown',
				id: userId
			};
		}

		// Populate server filter values
		for (const serverName of serverNames) {
			filterSets[1].values[serverName] = {
				label: serverName,
				id: serverName
			};
		}

		return filterSets;
	}

	let { usageStats, users, onClose, filters }: Props = $props();
	let filterInputs = $state<FilterSet[]>(generateFilters(usageStats, users, filters));

	$effect(() => {
		filterInputs = generateFilters(usageStats, users, filters);
	});

	function handleApplyFilters() {
		const url = new URL(page.url);

		// Clear existing query parameters
		url.search = '';

		// Preserve existing date and other filters
		if (typeof window !== 'undefined') {
			const currentUrl = new URL(window.location.href);
			const startTime = currentUrl.searchParams.get('startTime');
			const endTime = currentUrl.searchParams.get('endTime');

			if (startTime) url.searchParams.set('startTime', startTime);
			if (endTime) url.searchParams.set('endTime', endTime);
		}

		for (const filterInput of filterInputs) {
			if (filterInput.selected) {
				let paramName = filterInput.property;
				if (paramName === 'mcpServerDisplayName') {
					paramName = 'name';
				}
				url.searchParams.set(paramName, filterInput.selected.toString());
			}
		}

		goto(url.toString());
	}
</script>

<div class="dark:border-surface3 h-full w-screen border-l border-transparent md:w-sm">
	<div class="relative w-full text-center">
		<h4 class="p-4 text-xl font-semibold">Filters</h4>
		<button class="icon-button absolute top-1/2 right-4 -translate-y-1/2" onclick={onClose}>
			<X class="size-5" />
		</button>
	</div>
	<div
		class="default-scrollbar-thin flex h-[calc(100%-60px)] flex-col gap-4 overflow-y-auto p-4 pt-0"
	>
		{#each filterInputs as filterInput, index (filterInput.property)}
			{@const options = Object.values(filterInput.values)}
			{#if options.length > 0}
				<div class="mb-2 flex flex-col gap-1">
					<label for={filterInput.property} class="text-md font-light">
						By {filterInput.label}
					</label>
					<Select
						class="dark:border-surface3 bg-surface1 border border-transparent shadow-inner dark:bg-black"
						classes={{
							root: 'w-full',
							clear: 'hover:bg-surface3 bg-transparent'
						}}
						{options}
						selected={filterInput.selected}
						onSelect={(option) => {
							const updatedFilterInputs = [...filterInputs];
							updatedFilterInputs[index].selected = option.id.toString();
							filterInputs = updatedFilterInputs;
						}}
						onClear={() => {
							const updatedFilterInputs = [...filterInputs];
							updatedFilterInputs[index].selected = '';
							filterInputs = updatedFilterInputs;
						}}
						position="top"
					/>
				</div>
			{/if}
		{/each}
		<div class="mt-auto">
			<button
				class="button-primary text-md w-full rounded-lg px-4 py-2"
				onclick={handleApplyFilters}>Apply Filters</button
			>
		</div>
	</div>
</div>
