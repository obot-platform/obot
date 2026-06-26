<script lang="ts">
	import { Lock } from '@lucide/svelte';

	interface Props {
		title: string;
	}

	let { title }: Props = $props();
</script>

<h2 class="text-lg font-semibold">
	{title}
	<span class="pill-rounded nowrap font-light">
		<Lock class="size-3" /> Helm-Deployed
	</span>
</h2>
