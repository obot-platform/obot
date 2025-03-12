<script lang="ts">
	import { replaceState } from '$app/navigation';
	import EditMode from '$lib/components/EditMode.svelte';
	import Obot from '$lib/components/Obot.svelte';
	import { initLayout } from '$lib/context/layout.svelte';
	import { initToolReferences } from '$lib/context/toolReferences.svelte';
	import { profile } from '$lib/stores';

	let { data } = $props();
	let project = $state(data.project);
	let tools = $state(data.tools ?? []);
	let currentThreadID = $state<string | undefined>(
		(typeof window !== 'undefined' && new URL(window.location.href).searchParams.get('thread')) ||
			undefined
	);
	let title = $derived(project?.name || 'Obot');

	initToolReferences(data.toolReferences ?? []);

	initLayout({
		sidebarOpen: true,
		// typeof window !== 'undefined' && new URL(window.location.href).searchParams.has('sidebar'),
		projectEditorOpen:
			typeof window !== 'undefined' && new URL(window.location.href).searchParams.has('edit'),
		items: []
	});

	$effect(() => {
		// This happens on page transitions
		if (data.project?.id !== project?.id) {
			project = data.project;
			tools = data.tools ?? [];
			currentThreadID =
				(typeof window !== 'undefined' &&
					new URL(window.location.href).searchParams.get('thread')) ||
				undefined;
		}
	});

	$effect(() => {
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
</script>

<svelte:head>
	{#if title}
		<title>{title}</title>
	{/if}
</svelte:head>

<div class="h-svh">
	{#if project}
		{#key project.id}
			{#if project.editor}
				<EditMode bind:project bind:tools bind:currentThreadID />
			{:else}
				<Obot {project} {tools} bind:currentThreadID />
			{/if}
		{/key}
	{/if}
</div>
