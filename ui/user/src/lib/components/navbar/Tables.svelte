<script lang="ts">
	import { ChatService, EditorService, type Project, type TableList } from '$lib/services';
	import Menu from '$lib/components/navbar/Menu.svelte';
	import { Table } from 'lucide-svelte';
	import type { EditorItem } from '$lib/services/editor/index.svelte';
	import { getLayout } from '$lib/context/layout.svelte';
	import Truncate from '$lib/components/shared/tooltip/Truncate.svelte';

	interface Props {
		project: Project;
		items: EditorItem[];
	}

	async function loadTables() {
		tables = await ChatService.listTables(project.assistantID, project.id);
	}

	let { project, items = $bindable() }: Props = $props();

	let menu: ReturnType<typeof Menu>;
	let tables: TableList | undefined = $state();
	const layout = getLayout();
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
									await EditorService.load(items, project, 'table://' + table.name);
									layout.fileEditorOpen = true;
									menu?.toggle(false);
								}}
							>
								<Table class="h-5 w-5 min-w-fit" />
								<Truncate class="ms-2 group-hover:underline" text={table.name} />
							</button>
						</div>
					</li>
				{/each}
			</ul>
		{/if}
	{/snippet}
</Menu>
