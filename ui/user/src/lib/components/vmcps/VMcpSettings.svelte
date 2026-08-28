<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Search from '$lib/components/Search.svelte';
	import Select from '$lib/components/Select.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { VMCP_SORT_OPTIONS } from '$lib/services/vmcps/constants';
	import type { VMcpFilterOption, VMcpSortBy } from '$lib/services/vmcps/types';
	import { parseSelectedFilterIds } from '$lib/services/vmcps/utils';
	import { Settings, X } from '@lucide/svelte';

	const BUTTON_ID = 'vmcp-settings-button';
	const SORT_LABEL_ID = 'vmcp-sort-by-label';
	const SERVER_FILTER_LABEL_ID = 'vmcp-filter-by-server-label';
	const selectClasses = 'min-h-8 py-1 text-sm bg-base-200 dark:bg-base-100 shadow-inner!';

	interface Props {
		showAllConnectors?: boolean;
		showMyVMcpsOnly?: boolean;
		sortBy?: VMcpSortBy;
		ownerFilterBy?: string;
		componentFilterBy?: string;
		componentFilterOptions?: VMcpFilterOption[];
	}

	let {
		showAllConnectors = $bindable(false),
		showMyVMcpsOnly = $bindable(false),
		sortBy = $bindable('name'),
		componentFilterBy = $bindable(''),
		ownerFilterBy = $bindable(''),
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

<div class="flex items-center gap-2">
	<IconButton
		class="bg-base-100/80 dark:bg-base-300/80 rounded-md border border-transparent p-2 shadow-sm hover:bg-base-300 dark:hover:bg-base-300"
		id={BUTTON_ID}
		tooltip={{ text: 'vMCPs Settings', placement: 'right' }}
		onclick={() => dialog?.open()}
	>
		<Settings class="size-4" />
	</IconButton>
	<div
		class="bg-base-100/80 dark:bg-base-300/80 rounded-md border border-transparent p-2 shadow-sm flex flex-col @md:flex-row items-center gap-2"
	>
		<Search
			compact
			class="text-xs w-fit @2xl:min-w-xs dark:bg-base-100 shadow-inner"
			value={ownerFilterBy}
			onChange={(value) => (ownerFilterBy = value)}
			placeholder="Search by user..."
		/>
		<div class="divider divider-horizontal mx-0"></div>
		<label class="text-xs flex items-center gap-1.5 shrink-0 mr-2">
			<input
				type="checkbox"
				class="checkbox checkbox-xs rounded-sm"
				bind:checked={showMyVMcpsOnly}
			/>
			Show only my vMCPs
		</label>
	</div>
</div>

<ResponsiveDialog bind:this={dialog} title="vMCPs Settings" class="md:w-md">
	<div class="flex flex-col">
		<label class="flex items-center gap-2 text-sm">
			<input type="checkbox" class="checkbox checkbox-xs" bind:checked={showAllConnectors} />
			Include All User-Created vMCPs
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
