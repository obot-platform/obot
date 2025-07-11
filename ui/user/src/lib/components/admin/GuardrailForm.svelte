<script lang="ts">
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { LoaderCircle, Plus, Trash2, X } from 'lucide-svelte';
	import { type Snippet } from 'svelte';
	import { fly } from 'svelte/transition';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Table from '../Table.svelte';
	import { goto } from '$app/navigation';
	import { clickOutside } from '$lib/actions/clickoutside';

	interface Props {
		topContent?: Snippet;
	}

	let { topContent }: Props = $props();
	const duration = PAGE_TRANSITION_DURATION;
	let guardrail = $state<{
		displayName: string;
		models: {
			modelProviderID: string;
			id: string;
			name: string;
		}[];
		urls: string[];
	}>({
		displayName: '',
		models: [],
		urls: []
	});

	let saving = $state<boolean | undefined>();
	let addModelDialog = $state<HTMLDialogElement>();

	let addUrlDialog = $state<HTMLDialogElement>();
	let addingUrl = $state<string>('');

	let urlsTableData = $derived(
		guardrail.urls.map((url) => ({
			id: url,
			url
		}))
	);

	function closeAddUrlDialog() {
		addingUrl = '';
		addUrlDialog?.close();
	}
</script>

<div
	class="flex h-full w-full flex-col gap-4"
	out:fly={{ x: 100, duration }}
	in:fly={{ x: 100, delay: duration }}
>
	<div class="flex grow flex-col gap-4" out:fly={{ x: -100, duration }} in:fly={{ x: -100 }}>
		{#if topContent}
			{@render topContent()}
		{/if}
		<h1 class="text-2xl font-semibold">Create Guardrail</h1>

		<div
			class="dark:bg-surface2 dark:border-surface3 rounded-lg border border-transparent bg-white p-4"
		>
			<div class="flex flex-col gap-6">
				<div class="flex flex-col gap-2">
					<label for="mcp-catalog-name" class="flex-1 text-sm font-light capitalize"> Name </label>
					<input
						id="mcp-catalog-name"
						bind:value={guardrail.displayName}
						class="text-input-filled mt-0.5"
					/>
				</div>
			</div>
		</div>

		<div class="flex flex-col gap-2">
			<div class="mb-2 flex items-center justify-between">
				<h2 class="text-lg font-semibold">Webhook URLs</h2>
				<div class="relative flex items-center gap-4">
					<button
						class="button-primary flex items-center gap-1 text-sm"
						onclick={() => {
							addUrlDialog?.showModal();
						}}
					>
						<Plus class="size-4" /> Add URL
					</button>
				</div>
			</div>

			<Table data={urlsTableData} fields={['url']} noDataMessage="No URLs added.">
				{#snippet actions(d)}
					<button
						class="icon-button hover:text-red-500"
						onclick={() => {
							guardrail.urls = guardrail.urls?.filter((url) => url !== d.id) ?? [];
						}}
						use:tooltip={'Remove URL'}
					>
						<Trash2 class="size-4" />
					</button>
				{/snippet}
			</Table>
		</div>

		<div class="flex flex-col gap-2">
			<div class="mb-2 flex items-center justify-between">
				<h2 class="text-lg font-semibold">Models</h2>
				<div class="relative flex items-center gap-4">
					<button
						class="button-primary flex items-center gap-1 text-sm"
						onclick={() => {
							addModelDialog?.showModal();
						}}
					>
						<Plus class="size-4" /> Add Model
					</button>
				</div>
			</div>
			<Table data={guardrail.models} fields={['name']} noDataMessage="No models added.">
				{#snippet actions(d)}
					<button
						class="icon-button hover:text-red-500"
						onclick={() => {
							guardrail.models = guardrail.models?.filter((model) => model.id !== d.id) ?? [];
						}}
						use:tooltip={'Remove Model'}
					>
						<Trash2 class="size-4" />
					</button>
				{/snippet}
			</Table>
		</div>
	</div>
	<div
		class="bg-surface1 sticky bottom-0 left-0 flex w-full justify-end gap-2 py-4 text-gray-400 dark:bg-black dark:text-gray-600"
		out:fly={{ x: -100, duration }}
		in:fly={{ x: -100 }}
	>
		<div class="flex w-full justify-end gap-2">
			<button
				class="button text-sm"
				onclick={() => {
					goto('/v2/admin/guardrails');
				}}
			>
				Cancel
			</button>
			<button
				class="button-primary text-sm disabled:opacity-75"
				onclick={async () => {
					// TODO:
				}}
			>
				{#if saving}
					<LoaderCircle class="size-4 animate-spin" />
				{:else}
					Save
				{/if}
			</button>
		</div>
	</div>
</div>

<dialog bind:this={addUrlDialog} use:clickOutside={closeAddUrlDialog} class="w-full max-w-md p-4">
	<h3 class="default-dialog-title">
		Add Webhook URL
		<button onclick={closeAddUrlDialog} class="icon-button">
			<X class="size-5" />
		</button>
	</h3>

	<div class="my-4 flex flex-col gap-1">
		<label for="url" class="flex-1 text-sm font-light capitalize">URL</label>
		<input id="url" bind:value={addingUrl} class="text-input-filled" />
	</div>

	<div class="flex w-full justify-end gap-2">
		<button class="button" disabled={saving} onclick={closeAddUrlDialog}>Cancel</button>
		<button
			class="button-primary"
			disabled={saving || !addingUrl}
			onclick={async () => {
				if (!addingUrl) {
					return;
				}

				guardrail.urls = [...(guardrail.urls ?? []), addingUrl];
				addingUrl = '';
				closeAddUrlDialog();
			}}
		>
			Add
		</button>
	</div>
</dialog>

<dialog
	bind:this={addModelDialog}
	use:clickOutside={() => addModelDialog?.close()}
	class="w-full max-w-md p-4"
>
	<h3 class="default-dialog-title">Add Model</h3>
</dialog>
