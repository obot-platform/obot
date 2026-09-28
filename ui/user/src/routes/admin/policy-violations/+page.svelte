<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import MessagePolicyViolationsView from '$lib/components/admin/MessagePolicyViolationsView.svelte';
</script>

<svelte:head>
	<title>Obot | Message Policy Violations</title>
</svelte:head>

<Layout
	title="Message Policy Violations"
	classes={{
		container: 'md:px-0 px-0 pt-0',
		childrenContainer: 'max-w-none',
		noSidebarTitle: 'pl-4 md:pl-8 mx-auto md:max-w-(--breakpoint-xl) pt-4'
	}}
>
	<MessagePolicyViolationsView />
</Layout>
