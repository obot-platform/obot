<script lang="ts">
	import { LoaderCircle } from 'lucide-svelte';
	import { fade } from 'svelte/transition';

	interface Props {
		show: boolean;
		text?: string;
	}

	let { show, text }: Props = $props();
</script>

{#if show}
	<div
		in:fade={{ duration: 200 }}
		class="fixed top-0 left-0 z-50 flex h-svh w-svw items-center justify-center bg-black/50"
	>
		<div
			class="dark:bg-surface2 dark:border-surface3 flex items-center gap-2 rounded-xl bg-white px-4 py-2 shadow-sm dark:border"
		>
			<LoaderCircle class="size-10 animate-spin " />
			<p class="text-xl font-semibold">{text ?? 'Loading...'}</p>
		</div>
	</div>
{/if}
