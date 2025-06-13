<script lang="ts">
	import SearchUsers from '$lib/components/admin/SearchUsers.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import ProjectMcpConfig from '$lib/components/mcp/ProjectMcpConfig.svelte';
	import Search from '$lib/components/Search.svelte';
	import Table from '$lib/components/Table.svelte';
	import { type MCPCatalogEntry } from '$lib/services/admin/types.js';
	import { formatTimeAgo } from '$lib/time';
	import { ChevronLeft, Plus, RefreshCcw, Trash2 } from 'lucide-svelte';
	import { fly } from 'svelte/transition';

	const mockSources: {
		id: string;
		name: string;
		updated: string;
		url: string;
		readonly?: boolean;
	}[] = [
		{
			id: '1',
			name: 'github.acme-corp.com/it/obot-catalog',
			url: 'https://github.acme-corp.com/it/obot-catalog.json',
			updated: new Date().toISOString(),
			readonly: true
		},
		{
			id: '2',
			name: 'github.acme-corp.com/it/test-catalog',
			url: 'https://github.acme-corp.com/it/test-catalog.json',
			updated: new Date().toISOString()
		}
	];

	const mockEntries: (MCPCatalogEntry & { source?: (typeof mockSources)[0] })[] = [
		{
			id: '1',
			name: 'Tavily',
			source: mockSources[0],
			type: 'hosted',
			deployments: 100,
			manifest: {
				env: [
					{
						key: 'TAVILY_API_KEY',
						description: 'Your Tavily APY key. Get one at https:/app.tavily.com/home'
					}
				],
				command: 'npx',
				args: ['-y', 'tavily-mcp@latest']
			},
			readonly: true
		},
		{
			id: '2',
			name: 'PayPal',
			source: mockSources[1],
			type: 'remote',
			deployments: 32,
			manifest: {
				url: 'https://mcp.paypal.com/sse'
			},
			readonly: true
		},
		{
			id: '3',
			name: 'Pinecone',
			manifest: {
				url: 'https://pinecone.acme-corp.com/mcp/assistants/**'
			},
			type: 'remote',
			deployments: 15
		}
	];

	const mockUserGroups: {
		id: number;
		name: string;
		type: 'user' | 'group';
		role: 'owner' | 'user';
	}[] = [
		{
			id: 1,
			name: 'Craig Jellick',
			type: 'user',
			role: 'owner'
		},
		{
			id: 2,
			name: 'Engineering',
			type: 'group',
			role: 'user'
		}
	];

	let { data } = $props();
	const { mcpCatalog } = data;
	let selectedEntry = $state<Omit<(typeof mockEntries)[0], 'id'> & { id?: string }>();
	let selectedSource = $state<Omit<(typeof mockSources)[0], 'id'> & { id?: string }>();

	let addUserGroupDialog = $state<ReturnType<typeof SearchUsers>>();

	let searchEntries = $state('');
	const duration = 200;
</script>

<Layout>
	{#if selectedEntry}
		{@render configureEntryScreen(selectedEntry)}
	{:else if selectedSource}
		{@render configureSourceScreen(selectedSource)}
	{:else}
		{@render configureCatalogScreen(mcpCatalog)}
	{/if}
</Layout>

{#snippet configureCatalogScreen(config: { id: number; name: string; entries: number })}
	<div class="flex flex-col gap-8 py-8" out:fly={{ x: -100, duration }} in:fly={{ x: -100 }}>
		<a
			href={`/v2admin/mcp-catalogs`}
			class="button-text flex -translate-x-1 items-center gap-2 p-0 text-lg font-light"
		>
			<ChevronLeft class="size-6" />
			Back to MCP Catalogs
		</a>

		<h1 class="text-2xl font-semibold">
			{config.name}
		</h1>

		<div class="flex flex-col gap-2">
			<div class="mb-2 flex items-center justify-between">
				<h2 class="text-lg font-semibold">Catalog Sources</h2>
				<div class="relative flex items-center gap-4">
					<button
						class="button-primary flex items-center gap-1 text-sm"
						onclick={() => {
							selectedSource = {
								name: '',
								url: '',
								updated: new Date().toISOString()
							};
						}}
					>
						<Plus class="size-4" /> Add Source
					</button>
				</div>
			</div>

			<Table
				data={mockSources}
				fields={['name', 'updated']}
				onSelectRow={(d) => (selectedSource = d)}
			>
				{#snippet actions(d)}
					<button
						class="icon-button hover:text-red-500"
						onclick={() => {
							console.log(d);
						}}
					>
						<Trash2 class="size-4" />
					</button>
				{/snippet}
			</Table>
		</div>

		<div class="flex flex-col gap-2">
			<div class="mb-2 flex items-center justify-between">
				<h2 class="text-lg font-semibold">Catalog Entries</h2>

				<div class="relative flex items-center gap-4">
					<button
						class="button-primary flex items-center gap-1 text-sm"
						onclick={() => {
							selectedEntry = {
								name: '',
								type: 'hosted',
								manifest: {
									env: [],
									command: '',
									args: [],
									headers: [],
									url: ''
								}
							};
						}}
					>
						<Plus class="size-4" /> Add Entry
					</button>
				</div>
			</div>

			<Search
				class="dark:bg-surface1 dark:border-surface3 bg-white shadow-sm dark:border"
				onChange={(val) => {
					searchEntries = val;
				}}
				placeholder="Search by name..."
			/>

			<Table
				data={mockEntries}
				fields={['name', 'type', 'deployments']}
				onSelectRow={(d) => (selectedEntry = d)}
			>
				{#snippet actions(d)}
					<button
						class="icon-button hover:text-red-500"
						onclick={() => {
							console.log(d);
						}}
					>
						<Trash2 class="size-4" />
					</button>
				{/snippet}
			</Table>
		</div>

		<div class="flex flex-col gap-2">
			<div class="mb-2 flex items-center justify-between">
				<h2 class="text-lg font-semibold">Users & Groups</h2>
				<div class="relative flex items-center gap-4">
					<button
						class="button-primary flex items-center gap-1 text-sm"
						onclick={() => {
							addUserGroupDialog?.show();
						}}
					>
						<Plus class="size-4" /> Add User/Group
					</button>
				</div>
			</div>
			<Table data={mockUserGroups} fields={['name', 'type', 'role']}>
				{#snippet actions(d)}
					<button
						class="icon-button hover:text-red-500"
						onclick={() => {
							console.log(d);
						}}
					>
						<Trash2 class="size-4" />
					</button>
				{/snippet}
			</Table>
		</div>
	</div>
{/snippet}

{#snippet configureEntryScreen(entry: typeof selectedEntry)}
	{#if entry}
		<div class="flex flex-col gap-6 py-8" in:fly={{ x: 100, delay: duration, duration }}>
			<button
				onclick={() => (selectedEntry = undefined)}
				class="button-text flex -translate-x-1 items-center gap-2 p-0 text-lg font-light"
			>
				<ChevronLeft class="size-6" />
				Back to {mcpCatalog?.name ?? 'Source'}
			</button>

			<ProjectMcpConfig
				{entry}
				onCreate={async (newEntry) => {}}
				onUpdate={async (updatedEntry) => {}}
				readonly={entry.readonly}
			/>
		</div>
	{/if}
{/snippet}

{#snippet configureSourceScreen(source: typeof selectedSource)}
	{#if source}
		<div class="flex flex-col gap-8 py-8" in:fly={{ x: 100, delay: duration, duration }}>
			<button
				onclick={() => (selectedSource = undefined)}
				class="button-text flex -translate-x-1 items-center gap-2 p-0 text-lg font-light"
			>
				<ChevronLeft class="size-6" />
				Back to {mcpCatalog?.name ?? 'Source'}
			</button>

			<h1 class="text-2xl font-semibold">
				{source.name || 'Create Catalog Source'}
			</h1>

			<div
				class="dark:bg-surface1 dark:border-surface3 rounded-lg border border-transparent bg-white p-4 shadow-sm"
			>
				<div class="flex flex-col gap-6">
					<div class="flex flex-col gap-2">
						<label for="catalog-source-name" class="flex-1 text-sm font-light capitalize">
							Source Name
						</label>
						<input
							id="catalog-source-name"
							bind:value={source.name}
							class="text-input-filled mt-0.5"
							disabled={source.readonly}
						/>
					</div>

					<div class="flex flex-col gap-2">
						<label for="catalog-source-url" class="flex-1 text-sm font-light capitalize">
							Source URL
						</label>
						<input
							id="catalog-source-url"
							bind:value={source.url}
							class="text-input-filled mt-0.5"
							disabled={source.readonly}
						/>
					</div>

					{#if source.id}
						<div class="flex flex-col gap-2">
							<label
								for="catalog-source-name"
								class="flex flex-1 items-center gap-4 text-sm font-light capitalize"
							>
								Last Refreshed

								<button
									class="button flex items-center gap-1 text-xs hover:bg-blue-500 hover:text-white"
									onclick={() => {}}
									disabled={source.readonly}
								>
									<RefreshCcw class="size-3" /> Refresh Source
								</button>
							</label>
							<p class="flex text-sm">
								{formatTimeAgo(source.updated).relativeTime}
							</p>
						</div>
					{/if}
				</div>
			</div>

			<div class="flex w-full justify-end gap-2">
				<button class="button">Cancel</button>
				<button class="button-primary">Save Source</button>
			</div>
		</div>
	{/if}
{/snippet}

<SearchUsers bind:this={addUserGroupDialog} onAdd={() => {}} />

<svelte:head>
	<title>Obot | {mcpCatalog?.name ?? 'MCP Catalog'}</title>
</svelte:head>
