<script lang="ts">
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Search from '$lib/components/Search.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import Loading from '$lib/icons/Loading.svelte';
	import type { SkillRepository } from '$lib/services/admin/types';
	import { isWebURL, setUrlParamAndUpdateUrl } from '$lib/url';
	import { Pencil, PencilRuler, RefreshCcw, Trash2, TriangleAlert } from '@lucide/svelte';
	import { fade } from 'svelte/transition';

	interface Props {
		skillRepositories: SkillRepository[];
		syncingIds: Set<string>;
		isAdminReadonly: boolean;
		onEdit: (repository: SkillRepository) => void;
		onDelete: (repositories: SkillRepository[]) => void;
		onSync: (id: string) => void;
		onOpenSyncError: (url: string, error: string) => void;
		onSelectRepository: (displayName: string) => void;
	}

	let {
		skillRepositories,
		syncingIds,
		isAdminReadonly,
		onEdit,
		onDelete,
		onSync,
		onOpenSyncError,
		onSelectRepository
	}: Props = $props();

	let query = $derived(page.url.searchParams.get('query') ?? '');
	let isSyncing = $derived(syncingIds.size > 0);
	let skillRepositoriesTableData = $derived(
		(query
			? skillRepositories.filter(
					(d) =>
						d.displayName.toLowerCase().includes(query.toLowerCase()) ||
						d.repoURL.toLowerCase().includes(query.toLowerCase())
				)
			: skillRepositories
		).map((d) => ({
			...d,
			isSyncing: syncingIds.has(d.id)
		}))
	);

	function updateSearchQuery(value: string) {
		setUrlParamAndUpdateUrl(page.url, 'query', value);
	}
</script>

<div class="flex min-h-full flex-col" in:fade={{ duration: PAGE_TRANSITION_DURATION }}>
	<div class="bg-base-200 dark:bg-base-100 sticky top-0 z-20 w-full">
		<div class="mb-2">
			<Search
				class="dark:bg-base-200 dark:border-base-400 bg-base-100 border border-transparent shadow-sm"
				value={query}
				onChange={updateSearchQuery}
				placeholder="Search sources..."
			/>
		</div>
	</div>

	{#if skillRepositories.length > 0}
		<Table
			data={skillRepositoriesTableData}
			fields={['displayName', 'repoURL']}
			headers={[
				{
					property: 'displayName',
					title: 'Name'
				},
				{
					property: 'repoURL',
					title: 'URL'
				}
			]}
			noDataMessage="No Git Source URLs added."
			setRowClasses={(d) => {
				if (d.syncError) {
					return 'bg-warning/10';
				}
				return '';
			}}
			onClickRow={(d) => {
				onSelectRepository(d.displayName);
			}}
			classes={{
				root: 'rounded-md shadow-sm'
			}}
			sortable={['displayName']}
		>
			{#snippet actions(d)}
				{#if !isAdminReadonly}
					<IconButton
						onclick={(e) => {
							e.stopPropagation();
							onEdit(d);
						}}
					>
						<Pencil class="size-4" />
					</IconButton>
					<IconButton
						variant="danger"
						onclick={(e) => {
							e.stopPropagation();
							onDelete([d]);
						}}
					>
						<Trash2 class="size-4" />
					</IconButton>
				{/if}
			{/snippet}
			{#snippet onRenderColumn(property, d)}
				{#if property === 'repoURL'}
					<div class="flex items-center gap-2">
						{#if isWebURL(d.repoURL)}
							<a
								href={d.repoURL}
								target="_blank"
								rel="noopener noreferrer external"
								class="text-link"
							>
								{d.repoURL}
							</a>
						{:else}
							<p>{d.repoURL}</p>
						{/if}
						{#if d.syncError}
							<button
								onclick={(e) => {
									e.stopPropagation();
									onOpenSyncError(d.repoURL, d.syncError ?? '');
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
				{:else}
					{d[property as keyof typeof d]}
				{/if}
			{/snippet}
			{#snippet tableSelectActions(currentSelected)}
				<div class="flex grow items-center justify-end gap-2 px-4 py-2">
					<button
						class="btn btn-secondary flex items-center gap-1 text-sm font-normal"
						onclick={() => {
							for (const id of Object.keys(currentSelected)) {
								onSync(id);
							}
						}}
						disabled={isAdminReadonly || isSyncing}
					>
						{#if isSyncing}
							<Loading class="size-4" /> Syncing...
						{:else}
							<RefreshCcw class="size-4" /> Sync
						{/if}
					</button>
					<button
						class="btn btn-secondary flex items-center gap-1 text-sm font-normal"
						onclick={() => {
							onDelete(Object.values(currentSelected));
						}}
						disabled={isAdminReadonly}
					>
						<Trash2 class="size-4" /> Delete
					</button>
				</div>
			{/snippet}
		</Table>
	{:else}
		<div class="my-12 flex w-md flex-col items-center gap-4 self-center text-center">
			<PencilRuler class="text-muted-content size-24 opacity-25" />
			<h4 class="text-muted-content text-lg font-semibold">No current Git Source URLs.</h4>
			<p class="text-muted-content text-sm font-light">
				Once a Git Source URL has been added, its <br />
				information will be quickly accessible here.
			</p>
		</div>
	{/if}
</div>
