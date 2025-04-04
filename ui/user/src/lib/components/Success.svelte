<script lang="ts">
	import { CheckCircle2, X } from 'lucide-svelte/icons';
	import { fade } from 'svelte/transition';

	interface Props {
		message: string;
		onClose?: () => void;
	}

	let { message, onClose }: Props = $props();
</script>

<div
	in:fade
	class="relative flex max-w-sm items-center gap-2 rounded-xl bg-gray-50 p-5 pr-12 dark:bg-gray-950"
>
	<div>
		<CheckCircle2 class="h-5 w-5 text-green-500" />
	</div>
	<div class="line-clamp-3 pr-5 text-sm font-normal break-words">
		{message}
	</div>
	{#if onClose}
		<button type="button" onclick={onClose} class="absolute top-0 right-0 p-4">
			<X class="h-5 w-5" />
		</button>
	{/if}
</div>
