<script lang="ts">
	import { Copy } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import { popover } from '$lib/actions';

	interface Props {
		text?: string;
		class?: string;
	}

	let { text, class: clazz = '' }: Props = $props();
	let message = $state<string>('Copy');
	let { ref, tooltip } = popover({
		placement: 'top-start',
		offset: 1,
		hover: true
	});

	function copy() {
		if (!text) return;
		if (!navigator.clipboard) return;

		navigator.clipboard.writeText(text);
		message = 'Copied!';
		setTimeout(() => {
			message = 'Copy';
		}, 750);
	}
</script>

{#if text}
	<div class="hidden" use:tooltip>
		<p
			class="rounded-lg bg-gray-800 px-2 py-1 text-sm text-gray-50 shadow dark:bg-gray-100 dark:text-black"
		>
			{message}
		</p>
	</div>
	<button use:ref onclick={() => copy()}>
		<Copy class={twMerge('h-4 w-4', clazz)} />
	</button>
{/if}
