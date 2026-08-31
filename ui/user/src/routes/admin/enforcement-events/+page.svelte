<script lang="ts">
	import Layout from '$lib/components/Layout.svelte';
	import EnforcementDecisionsContent from '$lib/components/admin/enforcement/EnforcementDecisionsContent.svelte';
	import VirtualPageRoot from '$lib/components/ui/virtual-page/virtual-page-viewport.svelte';
	import type { Component } from 'svelte';
	import { fade } from 'svelte/transition';
</script>

<svelte:head>
	<title>Obot | Enforcement Decisions</title>
</svelte:head>

<Layout
	title="Enforcement Decisions"
	subtitle="Device Management"
	classes={{ childrenContainer: 'max-w-none', container: 'pb-0' }}
	main={{
		component: VirtualPageRoot as unknown as Component,
		props: { class: '', as: 'main', itemHeight: 56, overscan: 5 }
	}}
>
	<div class="flex-1" in:fade={{ duration: 100 }}>
		<div class="flex min-h-full flex-col gap-6">
			<EnforcementDecisionsContent />
		</div>
	</div>
</Layout>
