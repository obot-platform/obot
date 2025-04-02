<script lang="ts">
	import { replaceState } from '$app/navigation';
	import { navigating } from '$app/state';
	import EditMode from '$lib/components/EditMode.svelte';
	import { initLayout } from '$lib/context/layout.svelte';
	import { initToolReferences } from '$lib/context/toolReferences.svelte';
	import { browser } from '$app/environment';
	import { profile, tools } from '$lib/stores';

	let { data } = $props();
	let project = $state(data.project);

	let currentThreadID = $state<string | undefined>(
		(browser && new URL(window.location.href).searchParams.get('thread')) || undefined
	);
	let title = $derived(project?.name || 'Obot');

	initToolReferences(data.toolReferences ?? []);

	initialLayout();

	tools.setTools(data.tools ?? []);
	tools.setMaxTools(data.assistant?.maxTools ?? 5);

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
			// Only call replaceState if we're not in the initial navigation
			if (!navigating) {
				replaceState(currentURL.toString(), {});
			}
		}
	});

	$effect(() => {
		if (profile.current.unauthorized) {
			// Redirect to the main page to log in.
			window.location.href = `/?rd=${window.location.pathname}`;
		}
	});

	function qIsSet(key: string): boolean {
		if (navigating?.to?.url.searchParams.has(key)) {
			return true;
		}
		return browser && new URL(window.location.href).searchParams.has(key);
	}

	function initialLayout() {
		initLayout({
			sidebarOpen: !qIsSet('edit') || qIsSet('sidebar'),
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
