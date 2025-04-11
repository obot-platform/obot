<script lang="ts">
	import { replaceState } from '$app/navigation';
	import { navigating } from '$app/state';
	import EditMode from '$lib/components/EditMode.svelte';
	import { initLayout } from '$lib/context/layout.svelte';
	import { initToolReferences } from '$lib/context/toolReferences.svelte';
	import { browser } from '$app/environment';
	import { profile, responsive } from '$lib/stores';
	import { qIsSet } from '$lib/url';
	import { initProjectTools } from '$lib/context/projectTools.svelte.js';

	let { data } = $props();
	let project = $state(data.project);

	let currentThreadID = $state<string | undefined>(
		(browser && new URL(window.location.href).searchParams.get('thread')) || undefined
	);
	let title = $derived(project?.name || 'Obot');

	initToolReferences(data.toolReferences ?? []);
	initialLayout();

	// Initialize project tools immediately
	initProjectTools({
		tools: data.tools ?? [],
		maxTools: data.assistant?.maxTools ?? 5
	});

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
		if (navigating) {
			initialLayout();
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
			replaceState(currentURL.toString(), {});
		}
	});

	$effect(() => {
		if (profile.current.unauthorized) {
			// Redirect to the main page to log in.
			window.location.href = `/?rd=${window.location.pathname}`;
		}
	});

	function initialLayout() {
		initLayout({
			sidebarOpen: (!qIsSet('edit') && !responsive.isMobile) || qIsSet('sidebar'),
			projectEditorOpen: qIsSet('edit'),
			items: []
		});
	}
</script>

<svelte:head>
	{#if title}
		<title>{title}</title>
	{/if}
</svelte:head>

<div class="h-svh">
	{#if project}
		{#key project.id}
			<EditMode bind:project bind:currentThreadID assistant={data.assistant} />
		{/key}
	{/if}
</div>
