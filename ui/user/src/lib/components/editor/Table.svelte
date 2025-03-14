<script lang="ts">
	import { ChatService, type Project, type Rows } from '$lib/services';
	import { RefreshCw, Table } from 'lucide-svelte';
	import Controls from '$lib/components/editor/Controls.svelte';
	import Input from '$lib/components/messages/Input.svelte';
	import type { EditorItem } from '$lib/services/editor/index.svelte';

	interface Props {
		tableName: string;
		project: Project;
		items: EditorItem[];
		currentThreadID?: string;
	}

	let { tableName, project, currentThreadID, items }: Props = $props();
	let data: Rows | undefined = $state<Rows>();
	let loading: Promise<Rows> | undefined = $state();

	async function loadData() {
		loading = ChatService.getRows(project.assistantID, project.id, tableName);
		data = await loading;
		loading = undefined;
	}

	$effect(() => {
		if (data === undefined && loading === undefined && tableName != '') {
			loadData();
		}
	});
</script>

<div class="flex size-full flex-col gap-5 pb-5 pr-5">
	<div class="flex items-center gap-2">
		<div class="flex items-center gap-2">
			<Table class="h-5 w-5" />
			<h3 class="text-lg font-semibold">{tableName}</h3>
		</div>
		<button
			class="flex items-center rounded-md p-3 text-gray hover:bg-gray-100 hover:dark:bg-gray-900"
			onclick={loadData}
		>
			{#if loading}
				<RefreshCw class="h-4 w-4 animate-spin" />
			{:else}
				<RefreshCw class="h-4 w-4" />
			{/if}
		</button>
		<div class="grow"></div>
		<Controls {project} />
	</div>
	<div class="w-full overflow-auto">
		<table class="w-full table-auto text-left">
			<thead class="bg-surface1">
				<tr>
					{#each data?.columns ?? [] as column}
						<th>{column}</th>
					{/each}
				</tr>
			</thead>
			<tbody>
				{#each data?.rows ?? [] as row}
					<tr>
						{#each data?.columns ?? [] as col}
							<td>{row[col]}</td>
						{/each}
					</tr>
				{/each}
			</tbody>
		</table>
	</div>
	{#if currentThreadID}
		<p class="mt-10 text-gray">
			You can modify the data and the schema of this table by enter your instructions below.
		</p>
		<div class="grow px-2">
			<Input
				placeholder="Insert some sample data."
				onSubmit={async (i) => {
					if (!currentThreadID) {
						return;
					}
					await ChatService.invoke(project.assistantID, project.id, currentThreadID, {
						prompt: `In the database table '${tableName}' do the following instruction:\n${i.prompt}`
					});
				}}
				{items}
			/>
		</div>
	{/if}
</div>

<style lang="postcss">
	th,
	td {
		@apply border-collapse border border-surface3 p-2 px-4;
	}
</style>
