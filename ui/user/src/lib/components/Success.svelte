<script lang="ts">
	import { CircleCheckBig, X } from 'lucide-svelte/icons';
	import { fade } from 'svelte/transition';

	interface Props {
		message: string;
		onClose?: () => void;
	}

	let { message, onClose }: Props = $props();
</script>

<div
	in:fade
	out:fade={{ duration: 100 }}
	class="relative flex max-w-sm items-center gap-2 rounded-xl bg-base-200/50 p-5 pr-12"
>
	<div>
		<CircleCheckBig class="h-5 w-5 text-success" />
	</div>
	<div class="line-clamp-3 pr-5 text-sm font-normal wrap-break-word">
		{message}
	</div>
	{#if onClose}
		<button type="button" onclick={onClose} class="absolute top-0 right-0 p-4">
			<X class="h-5 w-5" />
		</button>
	{/if}
</div>
