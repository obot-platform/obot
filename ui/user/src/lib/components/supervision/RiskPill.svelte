<script lang="ts">
	import { m } from '$lib/i18n';
	import { riskClass } from '$lib/services/provenance';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		risk: number;
		class?: string;
		/** Shows the word "Risk" before the score. */
		labelled?: boolean;
	}

	let { risk, class: klass, labelled = false }: Props = $props();
</script>

<span
	class={twMerge('badge badge-sm badge-soft font-mono tabular-nums', riskClass(risk), klass)}
	aria-label={`${m.supervision_risk()} ${risk}`}
>
	{#if labelled}{m.supervision_risk()}&nbsp;{/if}{risk}
</span>
