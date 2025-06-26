<script lang="ts">
	import { type PageProps } from './$types';
	import { goto } from '$app/navigation';
	import { profile } from '$lib/stores';
	import Profile from '$lib/components/navbar/Profile.svelte';
	import AgentCopy from '$lib/components/agents/AgentCopy.svelte';
	import { onMount } from 'svelte';

	let { data }: PageProps = $props();

	onMount(async () => {
		if (profile.current.unauthorized) {
			// Redirect to the main page to log in.
			window.location.href = `/?rd=${window.location.pathname}`;
		}
	});
</script>

<div class="flex h-screen w-screen flex-col">
	<div
		class="bg-surface1 relative z-40 flex h-16 w-full items-center justify-between gap-4 p-3 shadow-md md:gap-8"
	>
		<div class="flex shrink-0 items-center gap-2">
			<img src="/user/images/obot-icon-blue.svg" class="h-8" alt="Obot icon" />
		</div>
		<div class="flex items-center">
			<Profile />
		</div>
	</div>
	<div class="flex flex-1 items-center justify-center p-4">
		<div class="bg-surface1 dark:bg-surface2 w-full max-w-xl p-6 md:rounded-xl">
			<AgentCopy inline={true} onBack={() => goto('/')} template={data.template} mcps={data.mcps} />
		</div>
	</div>
</div>

<svelte:head>
	<title>Copy Agent Template | Obot</title>
</svelte:head>
