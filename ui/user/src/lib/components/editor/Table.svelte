<script lang="ts">
	import { ChatService, type Rows } from '$lib/services';
	import { currentAssistant } from '$lib/stores';
	import { RefreshCw, Table } from 'lucide-svelte';
	import Controls from '$lib/components/editor/Controls.svelte';
	import Input from '$lib/components/messages/Input.svelte';

	interface Props {
		tableName: string;
	}

	let { tableName }: Props = $props();
	let data: Rows | undefined = $state<Rows>();
	let loading: Promise<Rows> | undefined = $state();

	async function loadData() {
		loading = ChatService.getRows($currentAssistant.id, tableName);
		data = await loading;
		loading = undefined;
	}

	$effect(() => {
		if (data === undefined && loading === undefined && tableName != '' && $currentAssistant.id) {
			loadData();
		}
	});
</script>

<div class="flex max-h-full max-w-full flex-col gap-5 p-5">
	<div class="flex items-center">
		<div class="flex items-center gap-2">
			<Table class="h-5 w-5" />
			<h3 class="text-lg font-semibold">{tableName}</h3>
		</div>
		<div class="flex-1 px-10">
			<Input
				placeholder="Modify table or data"
				onSubmit={async (i) => {
					await ChatService.invoke($currentAssistant.id, {
						prompt: `In the table database '${tableName}' do the following instruction:\n${i.prompt}`
					});
				}}
			/>
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
		<Controls />
	</div>
	<div class="overflow-auto">
		<table class="table-auto text-left">
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
