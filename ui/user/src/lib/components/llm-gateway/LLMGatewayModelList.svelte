<script lang="ts">
	import CopyButton from '$lib/components/CopyButton.svelte';
	import Search from '$lib/components/Search.svelte';
	import type { Model } from '$lib/services';
	import { ModelUsageLabels, type ModelUsage } from '$lib/services/admin/types';

	interface Props {
		models: Model[];
	}

	let { models }: Props = $props();

	let search = $state('');

	let filteredModels = $derived(
		models
			.filter((m) => {
				if (!search) return true;
				const lower = search.toLowerCase();
				return (
					m.name.toLowerCase().includes(lower) ||
					(m.displayName ?? '').toLowerCase().includes(lower) ||
					(m.usage ?? '').toLowerCase().includes(lower)
				);
			})
			.sort((a, b) => (a.displayName || a.name).localeCompare(b.displayName || b.name))
	);

	function usageLabel(usage: string): string {
		return ModelUsageLabels[usage as ModelUsage] || usage;
	}
</script>

<div class="flex flex-col gap-3">
	<Search
		compact
		class="dark:bg-base-200 dark:border-base-400 shadow-inner dark:border"
		onChange={(val) => (search = val)}
		value={search}
		placeholder="Search models..."
	/>

	{#if filteredModels.length === 0}
		<div class="text-muted-content py-4 text-center text-sm">
			{#if models.length === 0}
				No models available.
			{:else}
				No models match "{search}".
			{/if}
		</div>
	{:else}
		<ul class="flex flex-col">
			{#each filteredModels as model (model.id)}
				<li
					class="border-base-300 dark:border-base-400 flex items-center justify-between gap-3 border-b py-2 last:border-b-0"
				>
					<div class="flex min-w-0 flex-col gap-0.5">
						<div class="flex items-center gap-2">
							<span class="truncate font-mono text-sm">{model.name}</span>
							<CopyButton text={model.name} tooltipText="Copy model name" />
						</div>
						{#if model.displayName && model.displayName !== model.name}
							<span class="text-muted-content truncate text-xs">{model.displayName}</span>
						{/if}
					</div>
					{#if model.usage}
						<span
							class="bg-base-200 dark:bg-base-300 text-muted-content shrink-0 rounded px-2 py-0.5 text-xs"
						>
							{usageLabel(model.usage)}
						</span>
					{/if}
				</li>
			{/each}
		</ul>
	{/if}
</div>
