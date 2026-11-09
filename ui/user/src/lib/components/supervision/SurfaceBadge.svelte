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

	// Explicit colors: the theme's secondary and neutral are light grays that a soft badge
	// renders nearly invisible. The theme defines only some Tailwind colors (see app.css), so
	// stick to those.
	function colorClass(s: string) {
		switch (s) {
			case 'llm':
				return 'border-transparent! bg-blue-500/15! text-blue-700! dark:text-blue-300!';
			case 'mcp':
				return 'border-transparent! bg-success/15! text-success!';
			case 'device':
				return 'border-transparent! bg-orange-500/15! text-orange-500!';
			case 'egress':
				return 'border-transparent! bg-red-500/10! text-red-700! dark:text-red-300!';
			case 'copilot':
				return 'border-transparent! bg-gray-500/15! text-gray-700! dark:text-gray-300!';
			default:
				return 'badge-ghost';
		}
	}
</script>

<span
	class={twMerge('badge badge-sm whitespace-nowrap uppercase', colorClass(surface), klass)}
	title={surfaceLabel(surface)}
>
	{short ? surface : surfaceLabel(surface)}
</span>
