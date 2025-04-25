<script lang="ts">
	import { ChatService, EditorService, type Project, type TableList } from '$lib/services';
	import { RefreshCcw } from 'lucide-svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import { fade } from 'svelte/transition';
	import { overflowToolTip } from '$lib/actions/overflow';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import CollapsePane from '$lib/components/edit/CollapsePane.svelte';

	interface Props {
		project: Project;
		editor?: boolean;
	}

	async function loadTables() {
		loadingTables = ChatService.listTables(project.assistantID, project.id);
	}

	let { project, editor }: Props = $props();

	let loadingTables = $state<Promise<TableList>>();
	let loaded = $state(false);
	const layout = getLayout();

	$effect(() => {
		if (layout.sidebarOpen && !loaded) {
			loadTables();
			loaded = true;
		}
	});
</script>

{#if editor}
	<CollapsePane classes={{ header: 'pl-3 py-2', content: 'p-2' }} iconSize={5}>
		{#snippet header()}
			<span class="flex grow items-center gap-2 text-start text-sm font-extralight"> Tables </span>
		{/snippet}
		<div class="flex flex-col gap-4">
			{@render content()}
			<div class="flex justify-end">
				<button class="button flex items-center gap-1 text-xs" onclick={() => loadTables()}>
					<RefreshCcw class="size-4" /> Refresh
				</button>
			</div>
		</div>
	</CollapsePane>
{:else}
	<div class="flex w-full flex-col px-3">
		<div class="mb-1 flex items-center gap-1">
			<p class="text-sm font-semibold">Tables</p>
			<div class="grow"></div>
			<button class="icon-button" onclick={() => loadTables()} use:tooltip={'Refresh Tables'}>
				<RefreshCcw class="size-4" />
			</button>
		</div>
		{@render content()}
	</div>
{/if}

{#snippet content()}
	<div class="flex flex-col gap-4">
		{#if loadingTables}
			{#await loadingTables}
				<p in:fade class="text-gray text-center text-sm dark:text-gray-300">Loading...</p>
			{:then tables}
				{#if !tables.tables || tables.tables.length === 0}
					<p class="text-gray py-4 text-center text-sm font-light dark:text-gray-300">No tables</p>
				{:else}
					<ul>
						{#each tables.tables as table (table.name)}
							<li
								class="group hover:bg-surface3 flex min-h-9 items-center gap-3 rounded-md text-xs font-light"
							>
								<button
									class="h-full w-full p-2 text-start capitalize"
									use:overflowToolTip
									onclick={() => {
										EditorService.load(layout.items, project, 'table://' + table.name);
										layout.fileEditorOpen = true;
									}}>{table.name}</button
								>
							</li>
						{/each}
					</ul>
				{/if}
			{/await}
		{/if}
	</div>
{/snippet}
