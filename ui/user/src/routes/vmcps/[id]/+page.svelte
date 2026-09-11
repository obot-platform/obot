<script lang="ts">
	import VMcpDesigner from '$lib/components/vmcps/VMcpDesigner.svelte';

	let { data } = $props();
	let usersMap = $derived(new Map(data.users.map((user) => [user.id, user])));
</script>

<VMcpDesigner vmcp={data.vmcp} {usersMap} />
