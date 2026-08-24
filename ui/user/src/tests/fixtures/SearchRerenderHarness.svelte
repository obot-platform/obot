<script lang="ts">
	import Search from '$lib/components/Search.svelte';

	let { onChange }: { onChange: (value: string) => void } = $props();
	let renders = $state(0);
</script>

<Search value="" {onChange} class={`render-${renders}`} />
<button onclick={() => renders++}>Rerender</button>
