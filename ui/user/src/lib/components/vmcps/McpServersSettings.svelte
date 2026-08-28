<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Select from '$lib/components/Select.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { VMCP_IDS } from '$lib/services/vmcps/constants';
	import type { VMcpFilterOption } from '$lib/services/vmcps/types';
	import { parseSelectedFilterIds } from '$lib/services/vmcps/utils';
	import { Settings, X } from '@lucide/svelte';

	interface Props {
		settings: {
			showDeprecatedServers: boolean;
			filterBy: string;
		};
		filterOptions?: VMcpFilterOption[];
	}

	let { settings = $bindable(), filterOptions = [] }: Props = $props();
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let open = $state(false);
	let categoryDraft = $state<string | number | undefined>('');

	const selectClasses = 'min-h-8 py-1 text-sm bg-base-200 dark:bg-base-100 shadow-inner!';

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

<IconButton
	id={VMCP_IDS.SETTINGS_BUTTON_ID}
	tooltip={{ text: 'MCP Servers Settings', placement: 'left' }}
	aria-haspopup="dialog"
	aria-expanded={open}
	aria-controls={VMCP_IDS.SETTINGS_PANEL_ID}
	onclick={() => dialog?.open()}
>
	<Settings class="size-4" />
</IconButton>

<ResponsiveDialog
	bind:this={dialog}
	id={VMCP_IDS.SETTINGS_PANEL_ID}
	title="MCP Server Settings"
	class="md:w-md"
	onOpen={() => (open = true)}
	onClose={() => (open = false)}
>
	<div class="flex flex-col">
		<label class="flex items-center gap-2 text-sm">
			<input
				type="checkbox"
				class="checkbox checkbox-sm"
				bind:checked={settings.showDeprecatedServers}
			/>
			Include deprecated MCP servers
		</label>
		<label
			id={VMCP_IDS.FILTER_LABEL_ID}
			for="mcp-server-filter-by"
			class="divider mt-4 mb-2 text-xs uppercase"
		>
			Filter By Categories
		</label>
		<p class="text-muted-content mb-2 text-xs">
			Choose any combination of categories. A server is shown if it matches any selected category.
		</p>
		<Select
			id="mcp-server-filter-by"
			options={unusedOptions(filterOptions, settings.filterBy)}
			bind:selected={categoryDraft}
			searchInDropdown
			placeholder="Filter by category"
			searchPlaceholder="Search categories..."
			ariaLabelledby={VMCP_IDS.FILTER_LABEL_ID}
			class={selectClasses}
			classes={{ root: 'grow', option: 'text-sm' }}
			onSelect={(option) => {
				settings.filterBy = addFilter(settings.filterBy, String(option.id));
				categoryDraft = '';
			}}
		/>
		{@render filterPills(pillsFor(filterOptions, settings.filterBy), (id) => {
			settings.filterBy = removeFilter(settings.filterBy, id);
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
