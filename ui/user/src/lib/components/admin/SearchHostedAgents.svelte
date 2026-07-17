<script lang="ts">
	import type { HostedAgent, HostedAgentAccessPolicyResource } from '$lib/services/admin/types';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import Search from '../Search.svelte';
	import { Bot, Check } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		hostedAgents: HostedAgent[];
		onAdd: (resources: HostedAgentAccessPolicyResource[]) => void;
		exclude?: string[];
		title?: string;
		wildcardAvailable?: boolean;
	}

	let {
		hostedAgents,
		onAdd,
		exclude = [],
		title = 'Add Agents',
		wildcardAvailable = true
	}: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let query = $state('');
	let selected = $state<HostedAgentAccessPolicyResource[]>([]);
	let selectedSet = $derived(new Set(selected.map((item) => item.id)));

	const filteredAgents = $derived.by(() => {
		const items = hostedAgents.filter((agent) => !exclude.includes(agent.id));
		return query
			? items.filter(
					(agent) =>
						agent.name?.toLowerCase().includes(query.toLowerCase()) ||
						agent.description?.toLowerCase().includes(query.toLowerCase())
				)
			: items;
	});

	function toggleSelection(item: HostedAgentAccessPolicyResource) {
		if (selectedSet.has(item.id)) {
			selected = selected.filter((existing) => existing.id !== item.id);
		} else {
			selected = [...selected, item];
		}
	}

	function handleAdd() {
		onAdd(selected);
		dialog?.close();
	}

	export function open() {
		selected = [];
		query = '';
		dialog?.open();
	}

	export function close() {
		dialog?.close();
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	{title}
	class="h-full w-full overflow-visible md:h-[500px] md:max-w-md"
	classes={{ header: 'p-4 md:pb-0', content: 'min-h-inherit p-0' }}
>
	<div class="default-scrollbar-thin flex grow flex-col gap-4 overflow-y-auto pt-1">
		<div class="flex flex-col gap-2">
			<div class="px-4">
				<Search
					class="dark:bg-base-200 dark:border-base-400 shadow-inner dark:border"
					onChange={(val) => (query = val)}
					value={query}
					placeholder="Search agents..."
				/>
			</div>

			<div class="flex flex-col">
				{#if wildcardAvailable && !exclude?.includes('*')}
					<button
						class={twMerge(
							'hover:bg-base-300 dark:hover:bg-base-200 flex items-center justify-between gap-4 px-4 py-3 text-left',
							selectedSet.has('*') && 'bg-base-200/50'
						)}
						onclick={() => toggleSelection({ type: 'selector', id: '*' })}
					>
						<div class="flex items-center gap-2">
							<div class="flex flex-col">
								<p class="font-medium">All Agents</p>
								<span class="text-muted-content text-xs">
									Grants access to all current and future agents
								</span>
							</div>
						</div>
						<div class="flex size-6 items-center justify-center">
							{#if selectedSet.has('*')}
								<Check class="text-primary size-6" />
							{/if}
						</div>
					</button>
				{/if}

				{#each filteredAgents as agent (agent.id)}
					<button
						class={twMerge(
							'hover:bg-base-300 dark:hover:bg-base-200 flex items-center justify-between gap-4 px-4 py-3 text-left',
							selectedSet.has(agent.id) && 'bg-base-200/50'
						)}
						onclick={() => toggleSelection({ type: 'hostedAgent', id: agent.id })}
					>
						<div class="flex items-center gap-2">
							<div class="flex flex-col">
								<p class="font-medium">{agent.name}</p>
								<span class="text-muted-content line-clamp-1 text-xs">
									{agent.description || agent.image}
								</span>
							</div>
						</div>
						<div class="flex size-6 items-center justify-center">
							{#if selectedSet.has(agent.id)}
								<Check class="text-primary size-6" />
							{/if}
						</div>
					</button>
				{/each}
			</div>
		</div>
	</div>
	<div class="flex w-full flex-col justify-between gap-4 p-4 md:flex-row">
		<div class="flex items-center gap-1 font-light">
			{#if selected.length > 0}
				<Bot class="size-4" />
				{selected.length} Selected
			{/if}
		</div>
		<div class="flex items-center gap-2">
			<button class="btn btn-secondary w-full md:w-fit" onclick={() => dialog?.close()}>
				Cancel
			</button>
			<button class="btn btn-primary w-full md:w-fit" onclick={handleAdd}> Confirm </button>
		</div>
	</div>
</ResponsiveDialog>
