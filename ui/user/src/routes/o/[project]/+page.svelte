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
	import { initProjectMCPs } from '$lib/context/projectMcps.svelte.js';
	import Tutorial from '$lib/components/Tutorial.svelte';

	let { data } = $props();
	let project = $state(data.project);

	let currentThreadID = $state<string | undefined>(
		(browser && new URL(window.location.href).searchParams.get('thread')) || undefined
	);
	let title = $derived(project?.name || 'Obot');

	initToolReferences(data.toolReferences ?? []);
	initProjectMCPs(data.mcps ?? []);
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
			<!-- <Tutorial
				id="agent-tutorial-seen"
				steps={[
					{
						title: "Looks like it's your first time here!",
						description: "Let's get you started with a quick tour of your obot."
					},
					{
						title: 'Go Back Home',
						description: 'If you need to go back to your agents, click here.',
						elementId: 'navbar-home-link'
					},
					{
						title: 'Send a message',
						description: 'Click the send button to send a message to the obot.',
						elementId: 'thread-input'
					},
					{
						title: 'Edit basic details',
						description:
							'Click here to modify basic agent information such as its icon, name, and description',
						elementId: 'edit-basic-details-button'
					},
					{
						title: "Update your agent's MCP servers",
						description: 'Modify what MCP servers your agent is using here.',
						elementId: 'sidebar-mcp-servers'
					},
					{
						title: 'Create agent specific tasks',
						description: 'Click here to create a new task.',
						elementId: 'sidebar-tasks'
					},
					{
						title: 'Create a new thread',
						description: 'Click here to create a new thread.',
						elementId: 'sidebar-threads'
					},
					{
						title: 'Supply agent file knowledge',
						description: 'Your agent can learn from files you share. You can upload them here.',
						elementId: 'sidebar-knowledge'
					},
					{
						title: 'Add Starter Files',
						description:
							"A copy of each starter file will be added to every chat thread and task run when they're created.",
						elementId: 'sidebar-starter-files'
					},
					{
						title: 'Tutorial Complete',
						description: "And that's it! You're all set to start using your agent."
					}
				]}
			/> -->
		{/key}
	{/if}
</div>
