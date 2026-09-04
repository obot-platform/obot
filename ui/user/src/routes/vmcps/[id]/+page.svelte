<script lang="ts">
	import VMcpDesigner from '$lib/components/vmcps/VMcpDesigner.svelte';

	let { data } = $props();
</script>

<VMcpDesigner vmcp={data.vmcp} />
