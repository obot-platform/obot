<script lang="ts">
	import VMcpDesigner from '$lib/components/vmcps/VMcpDesigner.svelte';
	import { version } from '$lib/stores';

	let { data } = $props();
	let usersMap = $derived(new Map(data.users.map((user) => [user.id, user])));

	$effect(() => {
		if (data.latestVersion) {
			version.initialize(data.latestVersion);
		}
	});
</script>

<VMcpDesigner vmcp={data.vmcp} {usersMap} />
