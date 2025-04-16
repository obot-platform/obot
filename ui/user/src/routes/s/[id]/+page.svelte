<script lang="ts">
	import { profile, responsive } from '$lib/stores';
	import { type PageProps } from './$types';
	import { goto } from '$app/navigation';
	import { ChatService } from '$lib/services';
	import { onMount } from 'svelte';
	import Profile from '$lib/components/navbar/Profile.svelte';
	import { LoaderCircle } from 'lucide-svelte';

	let { data }: PageProps = $props();
	let showWarning = $state(false);

	onMount(async () => {
		if (profile.current.unauthorized) {
			// Redirect to the main page to log in.
			window.location.href = `/?rd=${window.location.pathname}`;
		} else if (data.featured || data.isOwner) {
			createProject();
		} else {
			showWarning = true;
		}
	});

	async function createProject() {
		const urlParams = new URLSearchParams(window.location.search);
		const project = await ChatService.createProjectFromShare(data.id, {
			create: urlParams.has('create')
		});
		goto(`/o/${project.id}`, { replaceState: true });
	}
</script>

<!-- Header -->
<div class="flex h-screen w-screen flex-col">
	<div
		class="bg-surface1 relative z-40 flex h-16 w-full items-center justify-between gap-4 p-3 shadow-md md:gap-8"
	>
		<div class="flex shrink-0 items-center gap-2">
			<a href="/catalog"
				><img src="/user/images/obot-icon-blue.svg" class="h-8" alt="Obot icon" /></a
			>
		</div>
		<div class="flex items-center">
			<Profile />
		</div>
	</div>

	{#if showWarning}
		<div class="flex grow items-center justify-center">
			<div
				class="bg-surface1 dark:bg-surface2 flex h-full w-full flex-col items-center justify-center gap-4 p-5 md:h-fit md:max-w-md md:rounded-xl"
			>
				<div class="flex max-w-sm grow flex-col gap-4 text-center md:grow-0">
					<h2 class="border-surface3 border-b pb-4 text-xl font-semibold">Shared Obot</h2>
					<p class="text-md">
						This obot was published by a third-party user and may include prompts or tools not
						reviewed or verified by our team. It could interact with external systems, access
						additional data sources, or behave in unexpected ways.
					</p>
					<p class="text-md">
						By continuing, you acknowledge that you understand the risks and choose to proceed at
						your own discretion.
					</p>
					{#if responsive.isMobile}
						<div class="flex grow"></div>
					{/if}
				</div>

				<button class="button-primary mt-2 w-full" onclick={createProject}>I Understand</button>
				<button class="button w-full" onclick={() => goto('/catalog')}>Go Back</button>
			</div>
		</div>
	{:else}
		<div class="flex grow items-center justify-center">
			<div class="size-6">
				<LoaderCircle class="text-blue size-6 animate-spin" />
			</div>
		</div>
	{/if}
</div>

<svelte:head>
	<title>Obot</title>
</svelte:head>
