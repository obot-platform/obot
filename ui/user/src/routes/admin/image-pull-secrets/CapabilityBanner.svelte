<script lang="ts">
	import { Info } from 'lucide-svelte';

	interface Props {
		reason?: string;
	}

	let { reason }: Props = $props();
</script>

<div class="notification-info flex items-center gap-3 text-sm">
	<Info class="size-5" />
	<div>
		<p class="font-semibold">Managed image pull secrets are unavailable.</p>
		{#if reason}
			<p>{reason}</p>
		{/if}
	</div>
</div>
