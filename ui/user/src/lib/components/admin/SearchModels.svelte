<script lang="ts">
	import { Check, Cpu, LoaderCircle } from 'lucide-svelte';
	import Search from '../Search.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import { twMerge } from 'tailwind-merge';
	import { ModelUsageLabels, type ModelUsage } from '$lib/services/admin/types';
	import { AdminService } from '$lib/services';
	import type { Model } from '$lib/services/chat/types';
	import { onMount } from 'svelte';

	interface Props {
		onAdd: (modelIds: string[]) => void;
		exclude?: string[];
		title?: string;
	}

	type SearchItem = {
		icon: string | undefined;
		iconDark: string | undefined;
		name: string;
		provider: string;
		usage: string;
		id: string;
		type: 'model' | 'all';
	};

	let { onAdd, exclude = [], title = 'Add Model(s)' }: Props = $props();
	let addModelDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let search = $state('');
	let selected = $state<SearchItem[]>([]);
	let selectedMap = $derived(new Set(selected.map((i) => i.id)));

	let models = $state<Model[]>([]);
	let loading = $state(true);

	onMount(async () => {
		models = await AdminService.listModels();
		loading = false;
	});

	let allData: SearchItem[] = $derived(
		[
			{
				icon: undefined,
				iconDark: undefined,
				name: 'All Models',
				provider: 'All Providers',
				usage: 'Grants access to all current and future models',
				id: '*',
				type: 'all' as const
			},
			...models.map((model) => ({
				icon: model.icon,
				iconDark: model.iconDark,
				name: model.displayName || model.name,
				provider: model.modelProviderName || '-',
				usage: ModelUsageLabels[model.usage as ModelUsage] || model.usage || '-',
				id: model.id,
				type: 'model' as const
			}))
		].filter((item) => !exclude?.includes(item.id))
	);

	let filteredData = $derived(
		search
			? allData.filter((item) => {
					return (
						item.name.toLowerCase().includes(search.toLowerCase()) ||
						item.provider.toLowerCase().includes(search.toLowerCase())
					);
				})
			: allData
	);

	export function open() {
		addModelDialog?.open();
	}

	function onClose() {
		search = '';
		selected = [];
	}

	function handleAdd() {
		const modelIds = selected.map((item) => item.id);
		onAdd(modelIds);
		addModelDialog?.close();
	}
</script>

<ResponsiveDialog
	bind:this={addModelDialog}
	{onClose}
	{title}
	class="h-full w-full overflow-visible md:h-[500px] md:max-w-md"
	classes={{ header: 'p-4 md:pb-0', content: 'min-h-inherit p-0' }}
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
						value={search}
						placeholder="Search by name or provider..."
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
								<div class="icon">
									{#if item.icon}
										<img
											src={item.icon}
											alt={item.name}
											class="size-8 flex-shrink-0 dark:hidden"
										/>
										<img
											src={item.iconDark || item.icon}
											alt={item.name}
											class="hidden size-8 flex-shrink-0 dark:block"
										/>
									{:else}
										<Cpu class="size-8 flex-shrink-0" />
									{/if}
								</div>
								<div class="flex min-w-0 grow flex-col">
									<div class="flex items-center gap-2">
										<p class="truncate font-medium">{item.name}</p>
										{#if item.type !== 'all'}
											<div class="dark:bg-surface2 bg-surface3 rounded-full px-3 py-1 text-[10px]">
												{item.provider}
											</div>
										{/if}
									</div>
									<span class="text-on-surface1 line-clamp-2 text-xs">
										{item.usage}
									</span>
								</div>
							</div>
							<div class="flex size-6 items-center justify-center">
								{#if selectedMap.has(item.id)}
									<Check class="text-primary size-6" />
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
				<Cpu class="size-4" />
				{selected.length} Selected
			{/if}
		</div>
		<div class="flex items-center gap-2">
			<button class="button w-full md:w-fit" onclick={() => addModelDialog?.close()}>
				Cancel
			</button>
			<button class="button-primary w-full md:w-fit" onclick={handleAdd}> Confirm </button>
		</div>
	</div>
</ResponsiveDialog>
