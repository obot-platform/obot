<script lang="ts">
	import { m } from '$lib/i18n';
	import { Info } from '@lucide/svelte';

	interface Props {
		reason?: string;
	}

	let { reason }: Props = $props();
</script>

<div class="notification-info flex items-center gap-3 text-sm">
	<Info class="size-5" />
	<div>
		<p class="font-semibold">{m.platform_capability_unavailable()}</p>
		{#if reason}
			<p>{reason}</p>
		{/if}
	</div>
</div>
