<script lang="ts">
	import { darkMode } from '$lib/stores';
	import { Bot } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		icon?: string;
		iconDark?: string;
		alt?: string;
		class?: string;
	}

	let { icon, iconDark, alt = '', class: klass }: Props = $props();

	// A dark variant is optional at every level, so fall back rather than
	// rendering nothing on a dark page.
	let src = $derived(darkMode.isDark ? iconDark || icon : icon);
</script>

{#if src}
	<img {src} {alt} class={twMerge('size-5 shrink-0 rounded-sm object-contain', klass)} />
{:else}
	<!-- A placeholder rather than a gap: rows stay aligned whether or not a
	     template declares an icon. -->
	<Bot class={twMerge('text-muted-content size-5 shrink-0 opacity-40', klass)} />
{/if}
