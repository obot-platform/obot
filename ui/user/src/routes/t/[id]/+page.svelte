<script lang="ts">
	import { type PageProps } from './$types';
	import { goto } from '$lib/url';
	import { profile } from '$lib/stores';
	import Profile from '$lib/components/navbar/Profile.svelte';
	import AgentCopy from '$lib/components/agents/AgentCopy.svelte';
	import { onMount } from 'svelte';
	import Logo from '$lib/components/Logo.svelte';

	let { data }: PageProps = $props();

	onMount(async () => {
		if (profile.current.unauthorized) {
			// Redirect to the main page to log in.
			window.location.href = `/?rd=${window.location.pathname}`;
		}
	});
</script>

<div class="bg-surface1 dark:bg-background flex h-dvh w-dvw flex-col">
	<div
		class="bg-surface1 relative z-40 flex h-16 w-full items-center justify-between gap-4 p-3 shadow-md md:gap-8"
	>
		<div class="flex shrink-0 items-center gap-2">
			<Logo />
		</div>
		<div class="flex items-center">
			<Profile />
		</div>
	</div>
	<div class="flex flex-1 items-center justify-center p-4">
		<div class="card">
			<AgentCopy onBack={() => goto('/')} template={data.template} />
		</div>
	</div>
</div>

<svelte:head>
	<title>Copy Project Snapshot | Obot</title>
</svelte:head>
