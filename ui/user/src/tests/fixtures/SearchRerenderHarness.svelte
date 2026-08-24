<script lang="ts">
	import Search from '$lib/components/Search.svelte';

	let {
		onChange,
		initialValue = ''
	}: { onChange: (value: string) => void; initialValue?: string } = $props();
	let renders = $state(0);
	// svelte-ignore state_referenced_locally
	let externalValue = $state(initialValue);
	let lastChange = '';

	function handleChange(value: string) {
		lastChange = value;
		onChange(value);
	}
</script>

<Search value={externalValue} onChange={handleChange} class={`render-${renders}`} />
<button onclick={() => renders++}>Rerender</button>
<button onclick={() => (externalValue = lastChange)}>Echo change</button>
