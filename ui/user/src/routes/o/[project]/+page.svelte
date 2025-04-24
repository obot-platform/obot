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
	import { ChatService, type MCP } from '$lib/services';
	import McpConfig from '$lib/components/mcp/McpConfig.svelte';
	import { onMount } from 'svelte';
	import { getProjectMCPs, initProjectMCPs } from '$lib/context/projectMcps.svelte.js';

	let { data } = $props();
	let project = $state(data.project);

	let currentThreadID = $state<string | undefined>(
		(browser && new URL(window.location.href).searchParams.get('thread')) || undefined
	);
	let title = $derived(project?.name || 'Obot');
	let mcpParam = $state('');
	let mcp = $state<MCP>();
	let mcpDialog = $state<ReturnType<typeof McpConfig>>();
	let mcpsContext = $state<ReturnType<typeof getProjectMCPs>>();

	initToolReferences(data.toolReferences ?? []);
	initProjectMCPs(data.mcps ?? []);
	initialLayout();

	// Initialize project tools immediately
	initProjectTools({
		tools: data.tools ?? [],
		maxTools: data.assistant?.maxTools ?? 5
	});

	onMount(() => {
		mcpParam = new URL(window.location.href).searchParams.get('mcp') ?? '';
		mcpsContext = getProjectMCPs();
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

	$effect(() => {
		if (mcpParam && data.assistant?.id && project?.id) {
			const existingProjectMcp = data.mcps.find((m) => m.catalogID === mcpParam);
			if (!existingProjectMcp) {
				ChatService.getMCP(mcpParam).then((response) => {
					mcp = response;
				});
			}
		}
	});

	$effect(() => {
		if (mcp) {
			mcpDialog?.open();
		}
	});

	function initialLayout() {
		initLayout({
			sidebarOpen: (!qIsSet('edit') && !responsive.isMobile) || qIsSet('sidebar'),
			projectEditorOpen: qIsSet('edit'),
			items: []
		});
	}

	async function handleMcpSubmit() {
		if (!data.assistant?.id || !project?.id || !mcp?.id) return;
		// TODO: handle values when endpoint is updated to support config keys

		const projectMcp = await ChatService.configureProjectMCP(data.assistant.id, project.id, mcp.id);
		if (mcpsContext) {
			mcpsContext.items = [...mcpsContext.items, projectMcp];
		}

		mcpDialog?.close();
		mcp = undefined;
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
	{#if mcp}
		<McpConfig
			bind:this={mcpDialog}
			{mcp}
			onSubmit={handleMcpSubmit}
			submitText="Start Chatting"
			disableOutsideClick
			hideCloseButton
		/>
	{/if}
</div>
