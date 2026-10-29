<script lang="ts">
	import { surfaceLabel } from '$lib/services/provenance';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		surface: string;
		class?: string;
		/** Renders the short surface key rather than its full name. */
		short?: boolean;
	}

	let { surface, class: klass, short = false }: Props = $props();

	function colorClass(s: string) {
		switch (s) {
			case 'llm':
				return 'badge-primary';
			case 'mcp':
				return 'badge-secondary';
			case 'device':
				return 'badge-accent';
			case 'egress':
				return 'badge-info';
			case 'copilot':
				return 'badge-neutral';
			default:
				return 'badge-ghost';
		}
	}
</script>

<span
	class={twMerge('badge badge-sm badge-soft uppercase', colorClass(surface), klass)}
	title={surfaceLabel(surface)}
>
	{short ? surface : surfaceLabel(surface)}
</span>
