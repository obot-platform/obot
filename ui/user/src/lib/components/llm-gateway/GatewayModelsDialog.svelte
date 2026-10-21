<script module lang="ts">
	import { m } from '$lib/i18n';
	import type { Model } from '$lib/services';
	import {
		providerIcon,
		providerIconClass,
		providerLabel
	} from '$lib/services/llm-gateway/providers';

	const FALLBACK_ICON = '/admin/assets/llm_gateway_icon.svg';

	export interface GatewayModelGroup {
		id: string;
		label: string;
		icon: string;
		// Extra classes for the icon, such as inverting a black logo in dark mode.
		iconClass: string;
		names: string[];
	}

	/** The distinct gateway names ("provider/model") of models, sorted. */
	export function gatewayModelNames(models: Model[]): string[] {
		return [...new Set(models.map((model) => model.targetModel ?? ''))]
			.filter(Boolean)
			.sort((a, b) => a.localeCompare(b));
	}

	/** The provider a gateway model name starts with, or '' for a name without one. */
	export function gatewayModelProvider(name: string): string {
		return name.includes('/') ? name.slice(0, name.indexOf('/')) : '';
	}

	// Models grouped under the provider their "provider/model" name starts with, both sorted
	// by name. Custom providers have no catalog entry, so they show their own name.
	export function groupGatewayModels(models: Model[], search = ''): GatewayModelGroup[] {
		const query = search.trim().toLowerCase();
		const groups: Record<string, string[]> = {};
		for (const name of gatewayModelNames(models)) {
			if (query && !name.toLowerCase().includes(query)) continue;
			(groups[gatewayModelProvider(name)] ??= []).push(name);
		}
		return Object.entries(groups)
			.map(([id, names]) => ({
				id,
				label: id ? providerLabel(id) : m.models_title(),
				icon: (id && providerIcon(id)) || FALLBACK_ICON,
				iconClass: id ? providerIconClass(id) : '',
				names
			}))
			.sort((a, b) => a.label.localeCompare(b.label));
	}
</script>

<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Search from '$lib/components/Search.svelte';

	interface Props {
		models: Model[];
	}

	let { models }: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let search = $state('');
	let groups = $derived(groupGatewayModels(models, search));

	export function open() {
		search = '';
		dialog?.open();
	}
</script>

<ResponsiveDialog bind:this={dialog} title={m.models_gateway_available_models()} class="max-w-3xl">
	<div class="flex max-h-[70vh] flex-col gap-3 overflow-y-auto text-sm px-0.5 pt-0.5">
		<Search
			compact
			value={search}
			class="bg-base-200 dark:bg-base-100"
			placeholder={m.models_gateway_search_models()}
			onChange={(v) => (search = v)}
		/>
		{#each groups as group (group.id)}
			<div class="flex flex-col gap-1.5">
				<h4 class="flex items-center gap-1.5 text-xs font-semibold">
					<img src={group.icon} alt="" class="size-4 object-contain {group.iconClass}" />
					{group.label}
				</h4>
				<div class="flex flex-wrap gap-1.5">
					{#each group.names as name (name)}
						<span class="bg-base-200 dark:bg-base-100 rounded px-2 py-0.5 font-mono text-xs"
							>{name}</span
						>
					{/each}
				</div>
			</div>
		{:else}
			<p class="text-muted-content text-xs">
				{m.models_gateway_no_models_match({ search: search.trim() })}
			</p>
		{/each}
	</div>
</ResponsiveDialog>
