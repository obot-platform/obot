<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Search from '$lib/components/Search.svelte';
	import Select from '$lib/components/Select.svelte';
	import { VMCP_SORT_OPTIONS } from '$lib/services/vmcps/constants';
	import type { VMcpFilterOption, VMcpSortBy } from '$lib/services/vmcps/types';
	import { parseSelectedFilterIds } from '$lib/services/vmcps/utils';
	import { Funnel, X } from '@lucide/svelte';

	const BUTTON_ID = 'vmcp-settings-button';
	const SORT_LABEL_ID = 'vmcp-sort-by-label';
	const SERVER_FILTER_LABEL_ID = 'vmcp-filter-by-server-label';
	const selectClasses = 'min-h-8 py-1 text-sm bg-base-200 dark:bg-base-100 shadow-inner!';

	interface Props {
		showMyVMcpsOnly?: boolean;
		sortBy?: VMcpSortBy;
		query?: string;
		componentFilterBy?: string;
		componentFilterOptions?: VMcpFilterOption[];
	}

	let {
		showMyVMcpsOnly = $bindable(false),
		sortBy = $bindable('name'),
		componentFilterBy = $bindable(''),
		query = $bindable(''),
		componentFilterOptions = []
	}: Props = $props();
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let componentDraft = $state<string | number | undefined>('');

	function addFilter(selected: string, id: string) {
		const ids = parseSelectedFilterIds(selected);
		if (ids.includes(id)) return selected;
		return [...ids, id].join(',');
	}

	function removeFilter(selected: string, id: string) {
		return parseSelectedFilterIds(selected)
			.filter((value) => value !== id)
			.join(',');
	}

	function unusedOptions(options: VMcpFilterOption[], selected: string) {
		const ids = new Set(parseSelectedFilterIds(selected));
		return options.filter((option) => !ids.has(String(option.id)));
	}

	function pillsFor(options: VMcpFilterOption[], selected: string) {
		return parseSelectedFilterIds(selected).map(
			(id) => options.find((option) => option.id === id) ?? { id, label: id }
		);
	}
</script>

<div class="bg-base-200 dark:bg-base-100 sticky top-16 left-0 z-20 w-full py-1">
	<div class="flex items-center gap-2">
		<Search
			value={query}
			onChange={(value) => (query = value)}
			placeholder="Search vMCPs..."
			class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
		/>
		<button class="btn btn-neutral h-12.5" id={BUTTON_ID} onclick={() => dialog?.open()}>
			<Funnel class="size-4" /> Filters
		</button>
	</div>
</div>

<ResponsiveDialog bind:this={dialog} title="vMCPs Settings" class="md:w-md">
	<div class="flex flex-col gap-2">
		<label class="flex items-center gap-1.5 w-fit text-sm">
			<input
				type="checkbox"
				class="checkbox checkbox-xs rounded-sm"
				bind:checked={showMyVMcpsOnly}
			/>
			Show my vMCPs only
		</label>

		<label id={SORT_LABEL_ID} for="vmcp-sort-by" class="divider my-2 text-xs uppercase">
			Sort By
		</label>
		<div class="flex gap-4 items-center">
			<Select
				id="vmcp-sort-by"
				options={VMCP_SORT_OPTIONS}
				bind:selected={sortBy}
				placeholder="Sort by"
				ariaLabelledby={SORT_LABEL_ID}
				class={selectClasses}
				classes={{ root: 'grow', option: 'text-sm' }}
			/>
		</div>

		<label
			id={SERVER_FILTER_LABEL_ID}
			for="vmcp-filter-by-server"
			class="divider my-2 text-xs uppercase"
		>
			Filter By MCP Servers
		</label>
		<Select
			id="vmcp-filter-by-server"
			options={unusedOptions(componentFilterOptions, componentFilterBy)}
			bind:selected={componentDraft}
			searchInDropdown
			placeholder="Filter by MCP server"
			searchPlaceholder="Search MCP servers..."
			ariaLabelledby={SERVER_FILTER_LABEL_ID}
			class={selectClasses}
			classes={{ root: 'grow', option: 'text-sm' }}
			onSelect={(option) => {
				componentFilterBy = addFilter(componentFilterBy, String(option.id));
				componentDraft = '';
			}}
		/>
		{@render filterPills(pillsFor(componentFilterOptions, componentFilterBy), (id) => {
			componentFilterBy = removeFilter(componentFilterBy, id);
		})}
	</div>
</ResponsiveDialog>

{#snippet filterPills(items: VMcpFilterOption[], onRemove: (id: string) => void)}
	{#if items.length > 0}
		<ul class="mt-2 flex flex-wrap gap-1">
			{#each items as item (item.id)}
				<li
					class="bg-base-400/50 dark:bg-base-400 inline-flex items-center gap-1 rounded-sm px-1.5 py-0.5 text-xs"
				>
					<span>{item.label}</span>
					<button
						type="button"
						class="btn btn-square btn-ghost size-4 min-h-4 text-muted-content hover:text-base-content"
						aria-label="Remove {item.label}"
						onclick={() => onRemove(String(item.id))}
					>
						<X class="size-3" />
					</button>
				</li>
			{/each}
		</ul>
	{/if}
{/snippet}
