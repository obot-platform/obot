<script lang="ts">
	import { m } from '$lib/i18n';
	import { Ban, RefreshCw, Search } from '@lucide/svelte';

	export interface CapabilityListItem {
		id: string;
		title: string;
		description?: string;
		metadata?: string;
	}

	interface Props {
		label: string;
		items: CapabilityListItem[];
		selectedId?: string;
		loading?: boolean;
		busy?: boolean;
		onselect: (id: string) => void;
		onrefresh: () => void;
		oncancel: () => void;
	}

	let {
		label,
		items,
		selectedId,
		loading = false,
		busy = false,
		onselect,
		onrefresh,
		oncancel
	}: Props = $props();
	let search = $state('');
	let filtered = $derived.by(() => {
		const query = search.trim().toLocaleLowerCase();
		if (!query) return items;
		return items.filter((item) =>
			[item.title, item.description, item.metadata]
				.filter(Boolean)
				.some((value) => value?.toLocaleLowerCase().includes(query))
		);
	});
</script>

<section
	class="flex flex-col gap-3 p-1 md:min-h-0"
	aria-label={m.mcps_tester_capability_list({ label })}
>
	<div class="flex shrink-0 flex-wrap gap-2">
		<label class="relative min-w-48 flex-1">
			<Search
				class="text-muted-content absolute top-1/2 left-3 size-4 -translate-y-1/2"
				aria-hidden="true"
			/>
			<span class="sr-only"
				>{m.mcps_tester_capability_search({ label: label.toLocaleLowerCase() })}</span
			>
			<input
				class="text-input-filled pl-9"
				bind:value={search}
				type="search"
				placeholder={m.mcps_tester_capability_search({ label: label.toLocaleLowerCase() })}
			/>
		</label>
		{#if loading}
			<button type="button" class="btn btn-secondary" onclick={oncancel}>
				<Ban class="size-4" aria-hidden="true" />
				{m.common_cancel()}
			</button>
		{:else}
			<button type="button" class="btn btn-secondary" disabled={busy} onclick={onrefresh}>
				<RefreshCw class="size-4" aria-hidden="true" />
				{m.mcps_tester_refresh()}
			</button>
		{/if}
	</div>

	{#if loading && items.length === 0}
		<p class="py-8 text-center text-sm text-muted-content" aria-live="polite">
			{m.mcps_tester_capability_loading({ label: label.toLocaleLowerCase() })}
		</p>
	{:else if filtered.length === 0}
		<p class="py-8 text-center text-sm text-muted-content">
			{search
				? m.mcps_tester_capability_no_match({ label: label.toLocaleLowerCase() })
				: m.mcps_tester_capability_none_found({ label: label.toLocaleLowerCase() })}
		</p>
	{:else}
		<!-- Capped on narrow screens; from md up it fills the height its column is given. -->
		<ul
			class="default-scrollbar-thin max-h-168 space-y-2 overflow-y-auto pr-1 md:max-h-none md:min-h-0 md:flex-1"
			aria-label={label}
		>
			{#each filtered as item (item.id)}
				<li>
					<button
						type="button"
						class={`hover:bg-base-200 dark:hover:bg-base-300 w-full rounded-lg border p-3 text-left disabled:cursor-not-allowed disabled:opacity-60 disabled:hover:bg-transparent dark:disabled:hover:bg-transparent ${
							selectedId === item.id
								? 'border-primary bg-primary/5'
								: 'border-base-300 dark:border-base-400'
						}`}
						disabled={busy}
						onclick={() => onselect(item.id)}
						aria-pressed={selectedId === item.id}
					>
						<strong class="block break-all text-sm">{item.title}</strong>
						{#if item.description}
							<span class="mt-1 line-clamp-2 block text-xs text-muted-content"
								>{item.description}</span
							>
						{/if}
						{#if item.metadata}
							<span class="mt-2 block text-xs text-muted-content">{item.metadata}</span>
						{/if}
					</button>
				</li>
			{/each}
		</ul>
	{/if}
</section>
