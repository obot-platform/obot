<script lang="ts">
	import { goto } from '$app/navigation';
	import Select from '$lib/components/Select.svelte';
	import type { AuditLog, AuditLogFilters } from '$lib/services/admin/types';
	import { X } from 'lucide-svelte';

	interface Props {
		auditLogs: (AuditLog & { user: string })[];
		onClose: () => void;
		filters?: AuditLogFilters;
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

	function generateFilters(logs: typeof auditLogs, filters?: AuditLogFilters) {
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
			},
			{
				label: 'Client',
				property: 'client',
				values: {},
				selected: filters?.client ?? ''
			},
			{
				label: 'Call Type',
				property: 'callType',
				values: {},
				selected: filters?.callType ?? ''
			},
			{
				label: 'Session ID',
				property: 'sessionId',
				values: {},
				selected: filters?.sessionId ?? ''
			}
		];

		for (const log of logs) {
			const { userID, mcpServerDisplayName, client, callType, sessionID, user } = log;

			if (userID) {
				filterSets[0].values[userID] = {
					label: user ?? 'Unknown',
					id: userID
				};
			}

			if (mcpServerDisplayName) {
				filterSets[1].values[mcpServerDisplayName] = {
					label: mcpServerDisplayName,
					id: mcpServerDisplayName
				};
			}

			if (client) {
				filterSets[2].values[client.name] = {
					label: client.name,
					id: client.name
				};
			}

			if (callType) {
				filterSets[3].values[callType] = {
					label: callType,
					id: callType
				};
			}

			if (sessionID) {
				filterSets[4].values[sessionID] = {
					label: sessionID,
					id: sessionID
				};
			}
		}

		return filterSets;
	}

	let { auditLogs, onClose, filters }: Props = $props();
	let filterInputs = $state<FilterSet[]>(generateFilters(auditLogs, filters));

	$effect(() => {
		if (filters || auditLogs) {
			filterInputs = generateFilters(auditLogs, filters);
		}
	});

	function handleApplyFilters() {
		const url = '/v2/admin/audit-logs';
		const params: string[] = [];
		for (const filterInput of filterInputs) {
			if (filterInput.selected) {
				params.push(
					`${filterInput.property}=${encodeURIComponent(filterInput.selected.toString())}`
				);
			}
		}

		if (params.length > 0) {
			goto(`${url}?${params.join('&')}`);
		} else {
			goto(url);
		}
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
