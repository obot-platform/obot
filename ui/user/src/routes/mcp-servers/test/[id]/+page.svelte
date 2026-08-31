<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Chat from '$lib/components/mcp/tester/Chat.svelte';
	import LogsInspector from '$lib/components/mcp/tester/LogsInspector.svelte';
	import PromptsInspector from '$lib/components/mcp/tester/PromptsInspector.svelte';
	import ResourcesInspector from '$lib/components/mcp/tester/ResourcesInspector.svelte';
	import ToolsInspector from '$lib/components/mcp/tester/ToolsInspector.svelte';
	import { VirtualPageViewport } from '$lib/components/ui/virtual-page';
	import { MCPTesterChat } from '$lib/services/mcp/tester-chat.svelte';
	import {
		MCPTesterSession,
		normalizeTesterSection,
		type TesterSection
	} from '$lib/services/mcp/tester.svelte';
	import { version } from '$lib/stores';
	import {
		TriangleAlert,
		ArrowLeft,
		KeyRound,
		MessageSquarePlus,
		RotateCw,
		Server
	} from '@lucide/svelte';
	import { onMount, type Component } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	let { data } = $props();
	let session = $state<MCPTesterSession>();
	let chat = $state<MCPTesterChat>();
	let confirmNewChat = $state(false);
	let activeSection = $derived(normalizeTesterSection(page.url.searchParams.get('tab')));
	let serverName = $derived(data.server.alias || data.server.manifest.name || data.server.id);
	let configuredDefault = $derived(
		data.defaultModelAliases?.find((alias) => alias.alias === 'llm')
	);
	let defaultModel = $derived(
		configuredDefault?.model
			? data.models?.find((model) => model.id === configuredDefault?.model && model.active)
			: undefined
	);
	let chatAvailable = $derived(Boolean(configuredDefault?.model && defaultModel));
	let chatUnavailableMessage = $derived(
		!configuredDefault?.model
			? 'No default llm model is configured. Configure one to use Chat.'
			: 'The configured default llm model is inactive or unavailable to your account.'
	);

	const sections: Array<{ id: TesterSection; label: string }> = [
		{ id: 'chat', label: 'Chat' },
		{ id: 'tools', label: 'Tools' },
		{ id: 'prompts', label: 'Prompts' },
		{ id: 'resources', label: 'Resources' },
		{ id: 'logs', label: 'MCP Log' }
	];

	const CARD_CLASS =
		'dark:bg-base-200 dark:border-base-400 bg-base-100 flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border border-transparent p-4 shadow-sm';

	function sectionURL(section: TesterSection): string {
		const url = new URL(page.url);
		url.searchParams.set('tab', section);
		return `/mcp-servers/test/${encodeURIComponent(data.server.id)}${url.search}`;
	}

	function showStagedChat(): void {
		void goto(resolve(sectionURL('chat') as `/${string}`));
	}

	function requestNewChat(): void {
		if (chat?.hasConversation) {
			confirmNewChat = true;
			return;
		}
		chat?.newChat();
	}

	function startNewChat(): void {
		confirmNewChat = false;
		chat?.newChat();
	}

	onMount(() => {
		const mountedSession = new MCPTesterSession(
			data.server,
			{
				name: 'obot-mcp-tester',
				version: version.current.obot || 'unknown'
			},
			fetch
		);
		session = mountedSession;
		chat = new MCPTesterChat(mountedSession, data.server.id, fetch);
		void mountedSession.initialize();
		return () => {
			chat?.close();
			mountedSession.close();
		};
	});
</script>

<Layout
	classes={{ container: 'min-h-0 overflow-hidden', childrenContainer: 'min-h-0' }}
	main={{
		component: VirtualPageViewport as unknown as Component,
		props: {
			class: 'overflow-hidden',
			as: 'main',
			itemHeight: 56,
			overscan: 5,
			disabled: true
		}
	}}
	title="MCP Tester"
>
	<div class="flex h-full min-h-0 flex-col gap-3">
		<header class="flex shrink-0 flex-wrap items-center justify-between gap-3">
			<div class="flex min-w-0 items-center gap-2">
				{#if data.server.manifest.icon}
					<img class="size-7 rounded-md object-contain" src={data.server.manifest.icon} alt="" />
				{:else}
					<div
						class="bg-base-200 dark:bg-base-300 flex size-7 items-center justify-center rounded-md"
					>
						<Server class="size-4" aria-hidden="true" />
					</div>
				{/if}
				<h1 class="min-w-0 truncate text-lg font-semibold">{serverName}</h1>
				<p class="shrink-0 text-xs text-muted-content">
					Status: {data.server.deploymentStatus ||
						(data.server.configured ? 'Configured' : 'Setup required')}
				</p>
			</div>
			<div class="flex shrink-0 items-center gap-2">
				{#if activeSection === 'chat' && chatAvailable && chat}
					<button type="button" class="btn btn-secondary btn-sm" onclick={requestNewChat}>
						<MessageSquarePlus class="size-4" aria-hidden="true" /> New Chat
					</button>
				{/if}
				<a class="btn btn-secondary btn-sm" href={resolve(data.backTarget as `/${string}`)}>
					<ArrowLeft class="size-4" aria-hidden="true" />
					Back to {serverName}
				</a>
			</div>
		</header>

		<nav
			class="border-base-300 dark:border-base-400 flex shrink-0 overflow-x-auto border-b"
			aria-label="MCP tester sections"
		>
			{#each sections as section (section.id)}
				<a
					class={twMerge(
						'page-tab min-w-fit py-2 text-center font-medium',
						activeSection === section.id && 'page-tab-active'
					)}
					href={resolve(sectionURL(section.id) as `/${string}`)}
					aria-current={activeSection === section.id ? 'page' : undefined}
				>
					{section.label}
					{#if section.id === 'chat' && chat?.approvalNeeded}
						<span class="badge badge-warning badge-sm ml-2">Approval needed</span>
					{/if}
				</a>
			{/each}
		</nav>

		{#if activeSection === 'logs'}
			<section class={CARD_CLASS}>
				<LogsInspector {session} {serverName} onretry={() => session?.initialize(true)} />
			</section>
		{:else if !session || session.status === 'idle' || session.status === 'initializing'}
			<section
				class="dark:bg-base-200 dark:border-base-400 bg-base-100 rounded-lg border border-transparent p-6 shadow-sm"
				aria-live="polite"
			>
				<h2 class="font-semibold">Connecting to {serverName}</h2>
				<p class="mt-1 text-sm text-muted-content">Opening an MCP session…</p>
			</section>
		{:else if session.status === 'access-denied'}
			<section class="notification-error p-6" role="alert">
				<TriangleAlert class="mb-2 size-5 text-error" aria-hidden="true" />
				<h2 class="font-semibold">Access denied</h2>
				<p class="mt-1 text-sm">
					Your permission to connect to this server is no longer available.
				</p>
				<a class="btn btn-secondary btn-sm mt-4" href={resolve(data.backTarget as `/${string}`)}
					>Back to server management</a
				>
			</section>
		{:else if session.status === 'reauthentication-required'}
			<section class="notification-alert p-6" role="status">
				<KeyRound class="mb-2 size-5 text-warning" aria-hidden="true" />
				<h2 class="font-semibold">Reauthentication required</h2>
				<p class="mt-1 text-sm">Reconnect this server before using the tester.</p>
				<a class="btn btn-primary btn-sm mt-4" href={resolve(data.backTarget as `/${string}`)}
					>Manage authentication</a
				>
			</section>
		{:else if session.status === 'setup-required'}
			<section class="notification-alert p-6" role="status">
				<TriangleAlert class="mb-2 size-5 text-warning" aria-hidden="true" />
				<h2 class="font-semibold">Server setup required</h2>
				<p class="mt-1 text-sm">Complete the server configuration before using the tester.</p>
				<a class="btn btn-primary btn-sm mt-4" href={resolve(data.backTarget as `/${string}`)}
					>Manage server</a
				>
			</section>
		{:else if session.status === 'unhealthy' || session.status === 'error'}
			<section class="notification-error p-6" role="alert">
				<TriangleAlert class="mb-2 size-5 text-error" aria-hidden="true" />
				<h2 class="font-semibold">Server unavailable</h2>
				<p class="mt-1 text-sm">{session.error || 'The server is not currently healthy.'}</p>
				<div class="mt-4 flex flex-wrap gap-2">
					<button class="btn btn-primary btn-sm" onclick={() => session?.initialize(true)}>
						<RotateCw class="size-4" aria-hidden="true" /> Retry
					</button>
					<a class="btn btn-secondary btn-sm" href={resolve(data.backTarget as `/${string}`)}
						>Manage server</a
					>
				</div>
			</section>
		{:else if session.status === 'ready'}
			<section class={CARD_CLASS}>
				{#if activeSection === 'chat'}
					{#if chatAvailable && chat}
						<Chat {chat} {session} />
					{:else}
						<h2 class="shrink-0 text-lg font-semibold">Chat</h2>
						<div class="bg-base-200 dark:bg-base-300 mt-4 rounded-lg p-4" role="status">
							<h3 class="font-medium">Chat unavailable</h3>
							<p class="mt-1 text-sm text-muted-content">{chatUnavailableMessage}</p>
						</div>
					{/if}
				{:else if activeSection === 'tools'}
					<ToolsInspector {session} />
				{:else if activeSection === 'prompts'}
					<PromptsInspector {session} onstaged={showStagedChat} />
				{:else if activeSection === 'resources'}
					<ResourcesInspector {session} onstaged={showStagedChat} />
				{/if}
			</section>
		{/if}
	</div>
</Layout>

<Confirm
	show={confirmNewChat}
	title="Start a new chat?"
	msg="Clear this ephemeral conversation?"
	note="Messages, staged context, approvals, and the frozen tool snapshot cannot be recovered."
	type="info"
	submitText="Start New Chat"
	onsuccess={startNewChat}
	oncancel={() => (confirmNewChat = false)}
/>

<svelte:head>
	<title>Obot | MCP Tester | {serverName}</title>
</svelte:head>
