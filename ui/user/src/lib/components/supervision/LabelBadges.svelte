<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { m } from '$lib/i18n';
	import type { LabelOrigin } from '$lib/services/provenance';
	import { formatTimeAgo } from '$lib/time';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		labels?: string[] | null;
		/** Where each label came from; shown as a tooltip when present. */
		origins?: Record<string, LabelOrigin[]>;
		max?: number;
		class?: string;
	}

	let { labels, origins, max, class: klass }: Props = $props();

	let shown = $derived(max ? (labels ?? []).slice(0, max) : (labels ?? []));
	let hidden = $derived((labels?.length ?? 0) - shown.length);

	function badgeClass(label: string) {
		switch (label) {
			case 'honeytoken':
			case 'secret':
				return 'badge-error';
			case 'untrusted':
				return 'badge-warning';
			default:
				return 'badge-primary';
		}
	}

	function originText(label: string) {
		const list = origins?.[label];
		if (!list?.length) return '';
		return list
			.slice(0, 4)
			.map((o) =>
				m.supervision_session_label_origin({
					summary: o.summary || o.eventID,
					time: formatTimeAgo(o.at).relativeTime
				})
			)
			.join('\n');
	}
</script>

{#if shown.length}
	<span class={twMerge('flex flex-wrap items-center gap-1', klass)}>
		{#each shown as label (label)}
			{@const text = originText(label)}
			{#if text}
				<span
					class={twMerge('badge badge-sm badge-soft cursor-help', badgeClass(label))}
					use:tooltip={{ text, classes: ['whitespace-pre-line', 'max-w-xs'] }}
				>
					{label}
				</span>
			{:else}
				<span class={twMerge('badge badge-sm badge-soft', badgeClass(label))}>{label}</span>
			{/if}
		{/each}
		{#if hidden > 0}
			<span class="badge badge-sm badge-ghost">+{hidden}</span>
		{/if}
	</span>
{/if}
