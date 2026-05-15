<script lang="ts">
	import { CircleCheckBig, X } from 'lucide-svelte/icons';
	import { fade } from 'svelte/transition';

	interface Props {
		message: string;
		onClose?: () => void;
	}

	let { message, onClose }: Props = $props();
</script>

<div in:fade out:fade={{ duration: 100 }} class="alert alert-horizontal alert-success alert-soft">
	<div>
		<CircleCheckBig class="size-5 text-success" />
	</div>
	<div class="line-clamp-3 text-sm font-normal wrap-break-word">
		{message}
	</div>
	{#if onClose}
		<button type="button" onclick={onClose}>
			<X class="size-4" />
		</button>
	{/if}
</div>
