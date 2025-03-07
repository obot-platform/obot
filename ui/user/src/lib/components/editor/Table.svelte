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

<div class="flex max-h-full max-w-full flex-col gap-5 py-5">
	<div class="flex items-center">
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
		{#if currentThreadID}
			<div class="grow px-2">
				<Input
					placeholder="Modify table or data"
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
		<Controls {project} />
	</div>
	<div class="w-full overflow-auto">
		<table class="w-full table-auto text-left">
			<thead class="bg-gray-50 dark:bg-gray-950">
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
</div>

<style lang="postcss">
	th,
	td {
		@apply border-collapse border border-gray p-2 px-4;
	}
</style>
