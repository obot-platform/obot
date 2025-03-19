<script lang="ts">
	import { ChatService, EditorService, type Project, type TableList } from '$lib/services';
	import { RefreshCcw } from 'lucide-svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import { fade } from 'svelte/transition';
	import { overflowToolTip } from '$lib/actions/overflow';

	interface Props {
		project: Project;
	}

	async function loadTables() {
		loadingTables = ChatService.listTables(project.assistantID, project.id);
	}

	let { project }: Props = $props();

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

<div class="flex w-full flex-col">
	<div class="mb-1 flex items-center gap-1">
		<p class="text-sm font-semibold">Tables</p>
		<div class="grow"></div>
		<button class="icon-button" onclick={() => loadTables()}>
			<RefreshCcw class="size-4" />
		</button>
	</div>
	<div>
		{#if loadingTables}
			{#await loadingTables}
				<p in:fade class="text-gray pt-6 pb-3 text-center text-sm dark:text-gray-300">Loading...</p>
			{:then tables}
				{#if !tables.tables || tables.tables.length === 0}
					<p class="text-gray pt-6 pb-3 text-center text-sm dark:text-gray-300">No tables</p>
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
</div>
