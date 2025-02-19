<script lang="ts">
	import type { Snippet } from 'svelte';
	import Loading from '$lib/icons/Loading.svelte';

	interface Props {
		children?: Snippet;
		class?: string;
	}

	let { children, class: clazz }: Props = $props();
	let spinning = $state(false);

	export function start() {
		spinning = true;
	}

	export function stop() {
		spinning = false;
	}
</script>

{#if spinning}
	{#if children}
		{@render children()}
	{:else}
		<Loading class={clazz} />
	{/if}
{/if}
