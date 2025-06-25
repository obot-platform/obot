<script lang="ts">
	import Navbar from '$lib/components/Navbar.svelte';
	import { columnResize } from '$lib/actions/resize';
	import { darkMode, profile, responsive } from '$lib/stores';
	import { initLayout, getLayout } from '$lib/context/layout.svelte';
	import type { Snippet } from 'svelte';
	import { fade, slide } from 'svelte/transition';
	import {
		Blocks,
		Bot,
		Boxes,
		Captions,
		Cpu,
		GlobeLock,
		LockKeyhole,
		MessageCircle,
		MessagesSquare,
		Puzzle,
		Server,
		SidebarClose,
		SidebarOpen,
		SquareArrowOutUpRight,
		Users
	} from 'lucide-svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { ChatService, EditorService } from '$lib/services';
	import { PROJECT_MCP_SERVER_NAME } from '$lib/constants';

	interface Props {
		children: Snippet;
	}

	const { children }: Props = $props();
	let nav = $state<HTMLDivElement>();

	initLayout();
	const layout = getLayout();

	async function handleOpenChat() {
		const projects = await ChatService.listProjects();
		const excludeMcpServer = projects.items.filter((p) => p.name !== PROJECT_MCP_SERVER_NAME);
		const lastVisitedObot = localStorage.getItem('lastVisitedObot');
		const lastProject =
			(lastVisitedObot && excludeMcpServer.find((p) => p.id === lastVisitedObot)) ??
			excludeMcpServer[excludeMcpServer.length - 1];
		if (lastProject) {
			window.open(`/o/${lastProject.id}`, '_blank');
		} else {
			const newProject = await EditorService.createObot();
			window.open(`/o/${newProject.id}`, '_blank');
		}
	}
</script>

<div class="flex min-h-dvh flex-col items-center">
	<div class="relative flex w-full grow">
		{#if layout.sidebarOpen}
			<div
				class="dark:bg-gray-990 flex w-screen min-w-screen flex-shrink-0 flex-col bg-white md:w-1/6 md:max-w-xl md:min-w-[250px]"
				transition:slide={{ axis: 'x' }}
				bind:this={nav}
			>
				<div class="flex h-16 flex-shrink-0 items-center px-3">
					{@render logo()}
				</div>

				<div class="text-md flex grow flex-col gap-8 px-3 pt-8 font-light">
					{#if profile.current?.isAdmin?.()}
						<div class="flex flex-col gap-1">
							<a href="/v2/admin/mcp-servers" class="sidebar-link">
								<Server class="size-4" /> MCP Servers
							</a>
							<a href="/v2/admin/access-control" class="sidebar-link">
								<GlobeLock class="size-4" /> Access Control
							</a>
							<a href="/v2/admin/audit-logs" class="sidebar-link">
								<Captions class="size-4" /> Audit Logs
							</a>
							<a href="/v2/admin/users" class="sidebar-link">
								<Users class="size-4" /> Users
							</a>
							<a href="/v2/admin/auth-providers" class="sidebar-link">
								<LockKeyhole class="size-4" /> Auth Providers
							</a>
						</div>
					{:else}
						<div class="flex flex-col gap-1">
							<a href="/mcp-servers" class="sidebar-link">
								<Server class="size-4" /> MCP Servers
							</a>
						</div>
					{/if}
				</div>

				<div class="flex justify-end px-3 py-2">
					<button
						use:tooltip={'Close Sidebar'}
						class="icon-button"
						onclick={() => (layout.sidebarOpen = false)}
					>
						<SidebarClose class="size-6" />
					</button>
				</div>
			</div>
			{#if !responsive.isMobile}
				<div
					role="none"
					class="h-inherit border-r-surface2 dark:border-r-surface2 relative -ml-3 w-3 cursor-col-resize border-r"
					use:columnResize={{ column: nav }}
				></div>
			{/if}
		{/if}

		<main
			class="bg-surface1 default-scrollbar-thin relative flex h-svh w-full grow flex-col overflow-y-auto dark:bg-black"
		>
			<Navbar class="dark:bg-gray-990 sticky top-0 left-0 z-30 w-full">
				{#snippet leftContent()}
					{#if !layout.sidebarOpen}
						{@render logo()}
					{/if}
				{/snippet}
			</Navbar>
			<div class="flex h-full flex-col items-center justify-center p-4 md:px-8">
				<div class="h-full w-full max-w-(--breakpoint-xl)">
					{@render children()}
				</div>
			</div>
		</main>
	</div>
	{#if !layout.sidebarOpen}
		<div class="absolute bottom-2 left-2 z-30" in:fade={{ delay: 300 }}>
			<button
				class="icon-button"
				onclick={() => (layout.sidebarOpen = true)}
				use:tooltip={'Open Sidebar'}
			>
				<SidebarOpen class="size-6" />
			</button>
		</div>
	{/if}
</div>

{#snippet logo()}
	<a href="/home" class="relative flex items-end">
		{#if darkMode.isDark}
			<img src="/user/images/obot-logo-blue-white-text.svg" class="h-12" alt="Obot logo" />
		{:else}
			<img src="/user/images/obot-logo-blue-black-text.svg" class="h-12" alt="Obot logo" />
		{/if}
		<div class="ml-1.5 -translate-y-1">
			<span
				class="rounded-full border-2 border-blue-400 px-1.5 py-[1px] text-[10px] font-bold text-blue-400 dark:border-blue-400 dark:text-blue-400"
			>
				BETA
			</span>
		</div>
	</a>
{/snippet}

<style lang="postcss">
	.sidebar-link {
		display: flex;
		width: 100%;
		align-items: center;
		gap: 0.5rem;
		border-radius: 0.375rem;
		padding: 0.5rem;
		transition: background-color 200ms;
		&:hover {
			background-color: var(--surface3);
		}
	}
</style>
