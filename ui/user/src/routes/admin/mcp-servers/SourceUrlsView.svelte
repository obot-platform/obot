<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { AdminService, type MCPCatalog } from '$lib/services';
	import { TriangleAlert, Link2, Pencil, Trash2 } from 'lucide-svelte';

	interface Props {
		catalog?: MCPCatalog;
		readonly?: boolean;
		onSync?: () => void;
		onEdit?: (url: string, index: number) => void;
		query?: string;
		syncing?: boolean;
	}
	let { catalog = $bindable(), readonly, onSync, onEdit, query }: Props = $props();

	let deletingSource = $state<{
		type: 'single' | 'multi';
		source?: string;
	}>();
	let selected = $state<string[]>([]);
	let deleting = $state(false);

	let syncError = $state<{ url: string; error: string }>();
	let syncErrorDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let tableData = $derived(
		catalog?.sourceURLs
			?.map((url) => ({ id: url, url }))
			?.filter((item) => item.url.toLowerCase().includes(query?.toLowerCase() ?? '')) ?? []
	);
</script>

<div class="flex flex-col gap-2">
	{#if catalog?.sourceURLs && catalog.sourceURLs.length > 0 && catalog.id}
		<Table
			data={tableData}
			fields={['url']}
			headers={[
				{
					property: 'url',
					title: 'URL'
				}
			]}
			noDataMessage="No Git Source URLs added."
			setRowClasses={(d) => {
				if (catalog?.syncErrors?.[d.url]) {
					return 'bg-warning/10';
				}
				return '';
			}}
			classes={{
				root: 'rounded-none rounded-b-md shadow-none'
			}}
		>
			{#snippet actions(d)}
				{#if !readonly}
					{#if onEdit}
						<IconButton
							onclick={() => {
								const index = catalog?.sourceURLs?.indexOf(d.url) ?? -1;
								onEdit(d.url, index);
							}}
						>
							<Pencil class="size-4" />
						</IconButton>
					{/if}
					<IconButton
						variant="danger"
						onclick={() => {
							deletingSource = { type: 'single', source: d.url };
						}}
					>
						<Trash2 class="size-4" />
					</IconButton>
				{/if}
			{/snippet}
			{#snippet onRenderColumn(property, d)}
				{#if property === 'url'}
					<div class="flex items-center gap-2">
						<p>{d.url}</p>
						{#if catalog?.syncErrors?.[d.url]}
							<button
								onclick={() => {
									syncError = {
										url: d.url,
										error: catalog?.syncErrors?.[d.url] ?? ''
									};
									syncErrorDialog?.open();
								}}
								use:tooltip={{
									text: 'An issue occurred. Click to see more details.',
									classes: ['wrap-break-word']
								}}
							>
								<TriangleAlert class="size-4 text-warning" />
							</button>
						{/if}
					</div>
				{/if}
			{/snippet}
			{#snippet tableSelectActions(currentSelected)}
				<div class="flex grow items-center justify-end gap-2 px-4 py-2">
					<button
						class="btn btn-secondary flex items-center gap-1 text-sm font-normal"
						onclick={() => {
							selected = Object.values(currentSelected).map((d) => d.url);
							deletingSource = { type: 'multi' };
						}}
						disabled={readonly}
					>
						<Trash2 class="size-4" /> Delete
					</button>
				</div>
			{/snippet}
		</Table>
	{:else}
		<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
			<Link2 class="text-base-content/80 size-24" />
			<h4 class="text-muted-content text-lg font-semibold">No current Git Source URLs.</h4>
			<p class="text-muted-content text-sm font-light">
				Once a Git Source URL has been added, its <br />
				information will be quickly accessible here.
			</p>
		</div>
	{/if}
</div>

<Confirm
	msg={deletingSource?.type === 'single'
		? 'Delete this Git Source URL?'
		: 'Delete selected Git Source URLs?'}
	show={Boolean(deletingSource)}
	onsuccess={async () => {
		if (!deletingSource || !catalog) {
			return;
		}

		deleting = true;
		let response;
		if (deletingSource.type === 'single') {
			response = await AdminService.updateMCPCatalog(catalog.id, {
				...catalog,
				sourceURLs: catalog.sourceURLs?.filter((url) => url !== deletingSource!.source)
			});
		} else {
			response = await AdminService.updateMCPCatalog(catalog.id, {
				...catalog,
				sourceURLs: catalog.sourceURLs?.filter((url) => !selected.includes(url))
			});
		}
		await onSync?.();
		catalog = response;
		deletingSource = undefined;
		deleting = false;
	}}
	oncancel={() => (deletingSource = undefined)}
	loading={deleting}
/>

<ResponsiveDialog title="Git Source URL Sync" bind:this={syncErrorDialog} class="md:w-2xl">
	<div class="mb-4 flex flex-col gap-4">
		<div class="notification-alert flex flex-col gap-2">
			<div class="flex items-center gap-2">
				<TriangleAlert class="size-6 shrink-0 self-start text-warning" />
				<p class="my-0.5 flex flex-col text-sm font-semibold">
					An issue occurred fetching this source URL:
				</p>
			</div>
			<span class="text-sm font-light break-all">{syncError?.error}</span>
		</div>
	</div>
</ResponsiveDialog>
