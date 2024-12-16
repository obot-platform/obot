<script lang="ts">
	import { currentAssistant } from '$lib/stores';
	import { ChatService, EditorService, type TableList } from '$lib/services';
	import Menu from '$lib/components/navbar/Menu.svelte';
	import { Database, Table } from 'lucide-svelte';

	async function loadTables() {
		tables = await ChatService.listTables($currentAssistant.id);
	}

	let menu: ReturnType<typeof Menu>;
	let tables = $state<TableList>({
		tables: []
	});
</script>

<Menu
	bind:this={menu}
	title="Tables"
	description="Click to view or edit table data"
	onLoad={loadTables}
>
	{#snippet icon()}
		<Database class="h-5 w-5" />
	{/snippet}
	{#snippet body()}
		{#if tables.tables.length === 0}
			<p class="pb-3 pt-6 text-center text-sm text-gray dark:text-gray-300">No files</p>
		{:else}
			<ul class="space-y-4 px-3 py-6 text-sm">
				{#each tables.tables as table}
					<li class="group">
						<div class="flex">
							<button
								class="flex flex-1 items-center"
								onclick={async () => {
									await EditorService.load($currentAssistant.id, 'table://' + table.name);
									menu?.open.set(false);
								}}
							>
								<Table class="h-5 w-5" />
								<span class="ms-3">{table.name}</span>
							</button>
						</div>
					</li>
				{/each}
			</ul>
		{/if}
	{/snippet}
</Menu>
