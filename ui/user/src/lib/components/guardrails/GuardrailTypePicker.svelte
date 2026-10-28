<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import { m } from '$lib/i18n';
	import {
		guardrailTypeDescription,
		guardrailTypeLabel
	} from '$lib/services/llm-gateway/guardrail-text';
	import {
		GUARDRAIL_CATEGORY_BUILT_IN,
		guardrailCategoryLabel,
		type Guardrail,
		type GuardrailProvider
	} from '$lib/services/llm-gateway/guardrails';

	interface Props {
		providers: GuardrailProvider[];
		/** Existing guardrails, to point out the integrations that are already configured. */
		guardrails?: Guardrail[];
		onSelect: (provider: GuardrailProvider) => void;
	}

	let { providers, guardrails = [], onSelect }: Props = $props();

	// Any number of guardrails can share a type, so a configured integration can still be
	// added again, for example with different settings; it's only marked.
	function configuredAs(p: GuardrailProvider) {
		if (p.category === GUARDRAIL_CATEGORY_BUILT_IN) return [];
		return guardrails.filter((g) => g.type === p.type).map((g) => g.name);
	}

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();

	export function open() {
		dialog?.open();
	}

	// Providers arrive sorted by category, then label.
	let categories = $derived(
		providers.reduce<{ name: string; providers: GuardrailProvider[] }[]>((acc, p) => {
			const last = acc[acc.length - 1];
			if (last?.name === p.category) last.providers.push(p);
			else acc.push({ name: p.category, providers: [p] });
			return acc;
		}, [])
	);
</script>

<ResponsiveDialog bind:this={dialog} title={m.guardrails_add()} class="max-w-3xl">
	<div class="flex max-h-[70vh] flex-col gap-4 overflow-y-auto p-4 md:p-0">
		{#each categories as c (c.name)}
			<div class="flex flex-col gap-2">
				<h3 class="text-muted-content text-xs font-semibold uppercase">
					{guardrailCategoryLabel(c.name)}
				</h3>
				<div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
					{#each c.providers as p (p.type)}
						{@const configured = configuredAs(p)}
						<button
							class="bg-base-200 hover:bg-base-300 dark:bg-base-100 dark:hover:bg-base-200 flex flex-col items-start gap-1 rounded-md p-3 text-left transition-colors"
							onclick={() => {
								dialog?.close();
								onSelect(p);
							}}
						>
							<span class="flex w-full items-center justify-between gap-2">
								<span class="text-sm font-medium">{guardrailTypeLabel(p)}</span>
								{#if configured.length > 0}
									<span
										class="badge badge-xs badge-soft shrink-0"
										use:tooltip={m.guardrails_type_configured_tooltip({
											names: configured.join(', ')
										})}
									>
										{m.guardrails_type_configured()}
									</span>
								{/if}
							</span>
							<span class="text-muted-content line-clamp-3 text-xs"
								>{guardrailTypeDescription(p)}</span
							>
						</button>
					{/each}
				</div>
			</div>
		{/each}
	</div>
</ResponsiveDialog>
