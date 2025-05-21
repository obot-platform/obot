<script lang="ts">
	import { LoaderCircle } from 'lucide-svelte';
	import type { Snippet } from 'svelte';
	import { fade, slide } from 'svelte/transition';
	import { onDestroy } from 'svelte';

	interface Props {
		show: boolean;
		text?: string;
		longLoadContent?: Snippet;
		longLoadDuration?: number;
	}

	let { show, text, longLoadContent, longLoadDuration = 30000 }: Props = $props();
	let isLongLoad = $state(false);
	let timeout = $state<ReturnType<typeof setTimeout>>();

	onDestroy(() => {
		if (timeout) {
			clearTimeout(timeout);
		}
	});

	$effect(() => {
		if (show) {
			if (!timeout) {
				timeout = setTimeout(() => {
					isLongLoad = true;
				}, longLoadDuration);
			}
		} else {
			isLongLoad = false;
			if (timeout) {
				clearTimeout(timeout);
				timeout = undefined;
			}
		}
	});
</script>

{#if show}
	<div
		in:fade={{ duration: 200 }}
		class="fixed top-0 left-0 z-50 flex h-svh w-svw items-center justify-center bg-black/50"
	>
		<div
			class="dark:bg-surface2 dark:border-surface3 flex flex-col items-center rounded-xl bg-white px-4 py-2 shadow-sm dark:border"
		>
			<div class="flex items-center gap-2">
				<LoaderCircle class="size-8 animate-spin " />
				<p class="text-xl font-semibold">{text ?? 'Loading...'}</p>
			</div>
			{#if isLongLoad && longLoadContent}
				<div in:slide>
					{@render longLoadContent()}
				</div>
			{/if}
		</div>
	</div>
{/if}
