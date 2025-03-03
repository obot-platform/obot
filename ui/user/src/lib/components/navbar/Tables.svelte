<script lang="ts">
	import { ChatService, EditorService, type TableList } from '$lib/services';
	import Menu from '$lib/components/navbar/Menu.svelte';
	import { Table } from 'lucide-svelte';
	import { popover } from '$lib/actions';

	async function loadTables() {
		tables = await ChatService.listTables();
	}

	let { tooltip, ref } = popover({ placement: 'top-start', offset: 10, hover: true });

	let menu: ReturnType<typeof Menu>;
	let tables: TableList | undefined = $state();
</script>

<Menu
	bind:this={menu}
	title="Tables"
	description="Click to view or edit table data"
	onLoad={loadTables}
>
	{#snippet icon()}
		<Table class="h-5 w-5" />
	{/snippet}
	{#snippet body()}
		{#if !tables}
			<p class="pb-3 pt-6 text-center text-sm text-gray dark:text-gray-300">Loading...</p>
		{:else if !tables.tables || tables.tables.length === 0}
			<p class="pb-3 pt-6 text-center text-sm text-gray dark:text-gray-300">No tables</p>
		{:else}
			<ul class="space-y-4 px-3 py-6 text-sm">
				{#each tables.tables as table}
					<li class="group">
						<div class="flex">
							<button
								class="flex flex-1 items-center"
								onclick={async () => {
									await EditorService.load('table://' + table.name);
									menu?.open.set(false);
								}}
							>
								<Table class="h-5 w-5" />
								<span class="ms-3" use:ref>
									{table.name.length > 25 ? table.name.slice(0, 25) + '...' : table.name}
								</span>

								<p
									use:tooltip
									class="max-w-md break-words rounded-xl bg-blue-500 p-2 text-white dark:text-black"
								>
									{table.name}
								</p>
							</button>
						</div>
					</li>
				{/each}
			</ul>
		{/if}
	{/snippet}
</Menu>
