<script lang="ts">
	import { Check, LoaderCircle, Server } from 'lucide-svelte';
	import Search from '../Search.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import { type AdminMcpServerAndEntriesContext } from '$lib/context/admin/mcpServerAndEntries.svelte';
	import { twMerge } from 'tailwind-merge';
	import { stripMarkdownToText } from '$lib/markdown';
	import { type PoweruserWorkspaceContext } from '$lib/context/poweruserWorkspace.svelte';
	import { ADMIN_ALL_OPTION } from '$lib/constants';
	import { getUserDisplayName } from '$lib/utils';
	import { AdminService, type OrgUser } from '$lib/services';
	import { KV, KVSync } from '$lib/kv';

	interface Props {
		onAdd: (mcpCatalogEntryIds: string[], mcpServerIds: string[], otherSelectors: string[]) => void;
		exclude?: string[];
		mcpEntriesContextFn?: () => AdminMcpServerAndEntriesContext | PoweruserWorkspaceContext;
		all?: { label: string; description: string };
		type: 'acr' | 'filter';
	}

	type SearchItem = {
		icon: string | undefined;
		name: string;
		description: string | undefined;
		id: string;
		type: 'mcpcatalogentry' | 'mcpserver' | 'all' | 'mcpCatalog';
		registry?: string;
	};

	const kv = KV.get();
	const kvSync = new KVSync(kv!);

	let { onAdd, exclude, mcpEntriesContextFn, all = ADMIN_ALL_OPTION, type }: Props = $props();
	let addMcpServerDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let users = $state<OrgUser[]>([]);
	let search = $state('');
	let selected = $state<SearchItem[]>([]);
	let selectedMap = $derived(new Set(selected.map((i) => i.id)));
	let usersMap = $derived(new Map(users?.map((user) => [user.id, user])));

	const mcpServerAndEntries = mcpEntriesContextFn?.() ?? {
		entries: [],
		servers: [],
		loading: false
	};

	let loading = $state(false);
	let allData: SearchItem[] = $derived(
		[
			{
				icon: undefined,
				name: all.label,
				description: all.description,
				id: type === 'acr' ? '*' : 'default',
				type: 'all' as const,
				registry: ''
			},
			...mcpServerAndEntries.entries.map((entry) => ({
				icon: entry.manifest?.icon,
				name: entry.manifest?.name || '',
				description: entry.manifest?.description,
				id: entry.id,
				type: 'mcpcatalogentry' as const,
				registry: entry.powerUserID
					? `${getUserDisplayName(usersMap, entry.powerUserID)}'s Registry`
					: ''
			})),
			...mcpServerAndEntries.servers.map((server) => ({
				icon: server.manifest.icon,
				name: server.manifest.name || '',
				description: server.manifest.description,
				id: server.id,
				type: 'mcpserver' as const,
				registry: server.userID ? `${getUserDisplayName(usersMap, server.userID)}'s Registry` : ''
			}))
		].filter((item) => !exclude?.includes(item.id))
	);
	let filteredData = $derived(
		search
			? allData.filter((item) => {
					return item.name.toLowerCase().includes(search.toLowerCase());
				})
			: allData
	);

	$effect.pre(() => {
		loadData();
	});

	export function open() {
		addMcpServerDialog?.open();
	}

	function onClose() {
		search = '';
		selected = [];
	}

	function handleAdd() {
		const mcpServerIds = [];
		const mcpCatalogEntryIds = [];
		const otherSelectors = [];
		for (const item of selected) {
			if (item.type === 'mcpserver') {
				mcpServerIds.push(item.id);
			} else if (item.type === 'mcpcatalogentry') {
				mcpCatalogEntryIds.push(item.id);
			} else {
				otherSelectors.push(item.id);
			}
		}
		onAdd(mcpCatalogEntryIds, mcpServerIds, otherSelectors);
		addMcpServerDialog?.close();
	}

	async function loadData() {
		try {
			loading = true;

			const u = await kvSync!.get(
				'users',
				() => AdminService.listUsersIncludeDeleted(),
				1000 * 60 * 5
			);

			users = u ?? [];

			loading = false;
		} catch (error) {
			console.error('Error initializing data:', error);
		}
	}
</script>

<ResponsiveDialog
	bind:this={addMcpServerDialog}
	{onClose}
	title="Add MCP Server(s)"
	class="h-full w-full overflow-visible p-0 md:h-[500px] md:max-w-md"
	classes={{ header: 'p-4 md:pb-0', content: 'min-h-inherit' }}
>
	<div class="default-scrollbar-thin flex grow flex-col gap-4 overflow-y-auto pt-1">
		<div class="flex flex-col gap-2">
			{#if loading}
				<div class="flex items-center justify-center">
					<LoaderCircle class="size-6 animate-spin" />
				</div>
			{:else}
				<div class="px-4">
					<Search
						class="dark:bg-surface1 dark:border-surface3 shadow-inner dark:border"
						onChange={(val) => (search = val)}
						placeholder="Search by name..."
					/>
				</div>

				<div class="flex flex-col">
					{#each filteredData as item (item.id)}
						<button
							class={twMerge(
								'dark:hover:bg-surface1 hover:bg-surface2 flex w-full items-center gap-2 px-4 py-2 text-left',
								selectedMap.has(item.id) && 'dark:bg-gray-920 bg-gray-50'
							)}
							onclick={() => {
								if (selectedMap.has(item.id)) {
									const index = selected.findIndex((i) => i.id === item.id);
									if (index !== -1) {
										selected.splice(index, 1);
									}
								} else {
									selected.push(item);
								}
							}}
						>
							<div class="flex w-full items-center gap-2 overflow-hidden">
								{#if item.icon}
									<img
										src={item.icon}
										alt={item.name}
										class="bg-surface1 size-8 flex-shrink-0 rounded-sm p-0.5 dark:bg-gray-600"
									/>
								{:else}
									<Server
										class="bg-surface1 size-8 flex-shrink-0 rounded-sm p-0.5 dark:bg-gray-600"
									/>
								{/if}
								<div class="flex min-w-0 grow flex-col">
									<div class="flex items-center gap-2">
										<p class="truncate">{item.name}</p>
										{#if item.registry}
											<div class="dark:bg-surface2 bg-surface3 rounded-full px-3 py-1 text-[10px]">
												{item.registry}
											</div>
										{/if}
									</div>
									<span class="line-clamp-2 text-xs text-gray-500">
										{@html stripMarkdownToText(item.description ?? '')}
									</span>
								</div>
							</div>
							<div class="flex size-6 items-center justify-center">
								{#if selectedMap.has(item.id)}
									<Check class="size-6 text-blue-500" />
								{/if}
							</div>
						</button>
					{/each}
				</div>
			{/if}
		</div>
	</div>
	<div class="flex w-full flex-col justify-between gap-4 p-4 md:flex-row">
		<div class="flex items-center gap-1 font-light">
			{#if selected.length > 0}
				<Server class="size-4" />
				{selected.length} Selected
			{/if}
		</div>
		<div class="flex items-center gap-2">
			<button class="button w-full md:w-fit" onclick={() => addMcpServerDialog?.close()}>
				Cancel
			</button>
			<button class="button-primary w-full md:w-fit" onclick={handleAdd}> Confirm </button>
		</div>
	</div>
</ResponsiveDialog>
