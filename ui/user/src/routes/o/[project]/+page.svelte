<script lang="ts">
	import { afterNavigate } from '$app/navigation';
	import { replaceState } from '$lib/url';
	import Obot from '$lib/components/Obot.svelte';
	import { getLayout, initLayout } from '$lib/context/chatLayout.svelte';
	import { initToolReferences } from '$lib/context/toolReferences.svelte';
	import { browser } from '$app/environment';
	import { profile, responsive } from '$lib/stores';
	import { qIsSet } from '$lib/url';
	import { initProjectTools } from '$lib/context/projectTools.svelte.js';
	import { initProjectMCPs } from '$lib/context/projectMcps.svelte.js';
	import { initHelperMode } from '$lib/context/helperMode.svelte.js';
	import { untrack } from 'svelte';

	let { data } = $props();
	let project = $state(untrack(() => data.project));

	let currentThreadID = $state<string | undefined>(
		(browser && new URL(window.location.href).searchParams.get('thread')) || undefined
	);
	let title = $derived(project?.name || 'Obot');

	untrack(() => {
		initToolReferences(data.toolReferences ?? []);
		initProjectMCPs(data.mcps ?? []);

		// Initialize project tools immediately
		initProjectTools({
			tools: data.tools ?? [],
			maxTools: data.assistant?.maxTools ?? 5
		});
	});

	initLayout({
		items: [],
		sidebarOpen: !responsive.isMobile
	});
	initHelperMode();

	// Update project tools when data changes
	$effect(() => {
		if (data.tools || data.assistant) {
			initProjectTools({
				tools: data.tools ?? [],
				maxTools: data.assistant?.maxTools ?? 5
			});
		}
	});

	$effect(() => {
		if (data.mcps) {
			initProjectMCPs(data.mcps);
		}
	});

	$effect(() => {
		// This happens on page transitions
		if (data.project?.id !== project?.id) {
			project = data.project;
			currentThreadID =
				(typeof window !== 'undefined' &&
					new URL(window.location.href).searchParams.get('thread')) ||
				undefined;
		}
	});

	$effect(() => {
		if (typeof window === 'undefined') return;

		const currentURL = new URL(window.location.href);
		if (
			currentThreadID &&
			project?.id &&
			currentURL.searchParams.get('thread') !== currentThreadID
		) {
			currentURL.searchParams.set('thread', currentThreadID);
			replaceState(currentURL, {});
		}
	});

	$effect(() => {
		if (profile.current.unauthorized) {
			// Redirect to the main page to log in.
			window.location.href = `/?rd=${window.location.pathname}`;
		}
	});

	const layout = getLayout();
	afterNavigate(() => {
		layout.sidebarOpen = !responsive.isMobile;
		layout.sidebarConfig = qIsSet('edit') ? 'project-configuration' : undefined;
		layout.items = [];
	});
</script>

<svelte:head>
	{#if title}
		<title>{title}</title>
	{/if}
</svelte:head>

<div class="h-svh">
	{#if project}
		{#key project.id}
			<div class="bg-surface1 flex size-full flex-col">
				<div class="flex grow overflow-auto">
					<div class="contents h-full grow border-r-0">
						<div class="size-full overflow-clip rounded-none transition-all">
							<Obot bind:project bind:currentThreadID assistant={data.assistant} />
						</div>
					</div>
				</div>
			</div>
		{/key}
	{/if}
</div>
