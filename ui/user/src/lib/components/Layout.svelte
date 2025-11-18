<script lang="ts">
	import Navbar from '$lib/components/Navbar.svelte';
	import { columnResize } from '$lib/actions/resize';
	import { profile, responsive, version } from '$lib/stores';
	import { initLayout, getLayout } from '$lib/context/layout.svelte';
	import { type Component, type Snippet } from 'svelte';
	import { fade, fly, slide } from 'svelte/transition';
	import {
		AlarmClock,
		Blocks,
		BookText,
		Boxes,
		Captions,
		ChartBarDecreasing,
		ChevronDown,
		ChevronLeft,
		ChevronUp,
		CircuitBoard,
		Cpu,
		Earth,
		ExternalLink,
		Funnel,
		GlobeLock,
		LockKeyhole,
		MessageCircle,
		MessageCircleMore,
		RadioTower,
		Server,
		ServerCog,
		Settings,
		SidebarClose,
		SidebarOpen,
		SquareLibrary,
		TowerControl,
		UserCog,
		Users
	} from 'lucide-svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { twMerge } from 'tailwind-merge';
	import BetaLogo from './navbar/BetaLogo.svelte';
	import ConfigureBanner from './admin/ConfigureBanner.svelte';
	import InfoTooltip from './InfoTooltip.svelte';
	import { Render } from './ui/render';
	import { ChatService, EditorService, Group } from '$lib/services';
	import { page } from '$app/state';
	import SetupSplashDialog from './admin/SetupSplashDialog.svelte';
	import { adminConfigStore } from '$lib/stores/adminConfig.svelte';
	import { workspaceStore } from '$lib/stores/workspace.svelte';
	import { goto } from '$app/navigation';
	import PageLoading from './PageLoading.svelte';

	type NavLink = {
		id: string;
		href?: string;
		icon: Component | typeof Server;
		label: string;
		disabled?: boolean;
		collapsible?: boolean;
		items?: NavLink[];
	};

	interface Props {
		classes?: {
			container?: string;
			childrenContainer?: string;
			navbar?: string;
		};
		children: Snippet;
		onRenderSubContent?: Snippet<[string]>;
		hideSidebar?: boolean;
		whiteBackground?: boolean;
		main?: { component: Component; props?: Record<string, unknown> };
		navLinks?: NavLink[];
		rightNavActions?: Snippet;
		title?: string;
		showBackButton?: boolean;
		onBackButtonClick?: () => void;
	}

	const {
		classes,
		children,
		navLinks: overrideNavLinks,
		onRenderSubContent,
		hideSidebar,
		whiteBackground,
		main,
		rightNavActions,
		title,
		showBackButton,
		onBackButtonClick
	}: Props = $props();
	let nav = $state<HTMLDivElement>();
	let collapsed = $state<Record<string, boolean>>({});
	let loadingChat = $state(false);
	let pathname = $derived(page.url.pathname);

	let workspace = $derived($workspaceStore);
	let isBootStrapUser = $derived(profile.current.isBootstrapUser?.() ?? false);
	let hasAdminAccess = $derived(profile.current.hasAdminAccess?.());
	let navLinks = $derived<NavLink[]>(
		overrideNavLinks ?? [
			...(workspace.rules.length > 0 || hasAdminAccess
				? [
						{
							id: 'mcp-registry',
							href: '/mcp-registry',
							icon: Server,
							label: 'MCP Registry',
							disabled: isBootStrapUser,
							collapsible: false
						},
						{
							id: 'mcp-registry-management',
							icon: SquareLibrary,
							label: 'MCP Registry Management',
							disabled: isBootStrapUser,
							collapsible: false,
							href: hasAdminAccess ? '/admin/access-control' : '/mcp-registry/created'
						}
					]
				: [
						{
							id: 'mcp-registry',
							href: '/mcp-registry',
							icon: Server,
							label: 'MCP Registry',
							disabled: isBootStrapUser,
							collapsible: false
						}
					]),
			{
				id: 'mcp-hosting',
				icon: RadioTower,
				label: 'MCP Hosting',
				disabled: isBootStrapUser,
				collapsible: hasAdminAccess,
				items: hasAdminAccess
					? [
							{
								id: 'servers',
								icon: Blocks,
								href: '/mcp-hosting',
								label: 'Servers',
								collapsible: false,
								disabled: isBootStrapUser
							},
							...(version.current.engine === 'kubernetes'
								? [
										{
											id: 'server-scheduling',
											href: '/admin/server-scheduling',
											icon: AlarmClock,
											label: 'Server Scheduling',
											collapsible: false,
											disabled: isBootStrapUser
										}
									]
								: [])
						]
					: []
			},
			{
				id: 'mcp-gateway',
				icon: Earth,
				label: 'MCP Gateway',
				disabled: isBootStrapUser,
				collapsible: hasAdminAccess,
				items: hasAdminAccess
					? [
							{
								id: 'audit-logs',
								href: '/admin/audit-logs',
								icon: Captions,
								label: 'Audit Logs',
								disabled: isBootStrapUser,
								collapsible: false
							},
							{
								id: 'usage',
								href: '/admin/usage',
								icon: ChartBarDecreasing,
								label: 'Usage',
								disabled: isBootStrapUser,
								collapsible: false
							},
							{
								id: 'filters',
								href: '/admin/filters',
								icon: Funnel,
								label: 'Filters',
								disabled: isBootStrapUser
							}
						]
					: [
							{
								id: 'audit-logs',
								href: '/mcp-gateway/audit-logs',
								icon: Captions,
								label: 'Audit Logs',
								disabled: isBootStrapUser,
								collapsible: false
							},
							{
								id: 'usage',
								href: '/mcp-gateway/usage',
								icon: ChartBarDecreasing,
								label: 'Usage',
								disabled: isBootStrapUser,
								collapsible: false
							}
						]
			},
			...(profile.current.hasAdminAccess?.()
				? [
						{
							id: 'chat-management',
							icon: MessageCircleMore,
							label: 'Chat Management',
							disabled: isBootStrapUser,
							collapsible: true,
							items: [
								{
									id: 'chat-threads',
									href: '/admin/chat-threads',
									icon: MessageCircleMore,
									label: 'Chat Threads',
									collapsible: false
								},
								{
									id: 'tasks',
									href: '/admin/tasks',
									icon: Cpu,
									label: 'Tasks',
									disabled: isBootStrapUser
								},
								{
									id: 'task-runs',
									href: '/admin/task-runs',
									icon: CircuitBoard,
									label: 'Task Runs',
									disabled: isBootStrapUser
								},
								{
									id: 'chat-configuration',
									href: '/admin/chat-configuration',
									icon: Settings,
									label: 'Chat Configuration',
									disabled: isBootStrapUser,
									collapsible: false
								},
								{
									id: 'model-providers',
									href: '/admin/model-providers',
									icon: Boxes,
									label: 'Model Providers',
									collapsible: false
								}
							]
						},
						{
							id: 'user-management',
							icon: Users,
							label: 'User Management',
							disabled: false,
							collapsible: true,
							items: [
								{
									id: 'users',
									href: '/admin/users',
									icon: Users,
									label: 'Users',
									collapsible: false,
									disabled: !version.current.authEnabled
								},
								{
									id: 'user-roles',
									href: '/admin/user-roles',
									icon: UserCog,
									label: 'User Roles',
									collapsible: false,
									disabled: !version.current.authEnabled
								},
								{
									id: 'auth-providers',
									href: '/admin/auth-providers',
									icon: LockKeyhole,
									label: 'Auth Providers',
									disabled: !version.current.authEnabled,
									collapsible: false
								}
							]
						}
					]
				: [])
		]
	);

	const tooltips = {
		'/admin/auth-providers': 'Enable authentication to access this page.'
	};

	$effect(() => {
		if (responsive.isMobile) {
			layout.sidebarOpen = false;
		}
	});

	const excludeConfigureBanner = ['/admin/model-providers', '/admin/auth-providers'];
	const isAdminRoute = $derived(pathname.includes('/admin'));

	$effect(() => {
		const isAdminOrBootstrapUser =
			profile.current.loaded &&
			(profile.current.groups.includes(Group.ADMIN) || profile.current.isBootstrapUser?.());

		workspaceStore.initialize();
		if (isAdminOrBootstrapUser && isAdminRoute) {
			adminConfigStore.initialize();
		}
	});

	initLayout();
	const layout = getLayout();

	function navigateTo(path: string, asNewTab?: boolean) {
		if (asNewTab) {
			// Create a temporary link element and click it; avoids Safari's popup blocker
			const link = document.createElement('a');
			link.href = path;
			link.target = '_blank';
			link.rel = 'noopener noreferrer';
			link.style.display = 'none';
			document.body.appendChild(link);
			link.click();
			document.body.removeChild(link);
		} else {
			goto(path);
		}
	}
</script>

<div class="flex min-h-dvh flex-col items-center">
	<div class="relative flex w-full grow">
		{#if layout.sidebarOpen && !hideSidebar}
			<div
				class="dark:bg-gray-990 flex max-h-dvh w-dvh min-w-dvw flex-shrink-0 flex-col bg-white md:w-1/6 md:max-w-xl md:min-w-[320px]"
				transition:slide={{ axis: 'x' }}
				bind:this={nav}
			>
				<div class="flex h-16 flex-shrink-0 items-center px-2">
					<BetaLogo enterprise={version.current.enterprise} />
				</div>

				<div
					class="text-md scrollbar-default-thin flex max-h-[calc(100vh-64px)] grow flex-col gap-8 overflow-y-auto px-3 pb-4 pl-2 font-medium"
				>
					<div class="flex flex-col gap-1">
						{#each navLinks as link (link.id)}
							<div class="flex">
								<div class="flex w-full items-center">
									{#if link.disabled}
										<div class="sidebar-link disabled">
											<link.icon class="size-5" />
											{link.label}
										</div>
									{:else if link.id === 'obot-chat'}
										<button
											class={twMerge(
												'sidebar-link',
												link.href && link.href === pathname && 'bg-surface2',
												!link.href && 'no-link'
											)}
											onclick={async () => {
												loadingChat = true;
												try {
													const projects = (await ChatService.listProjects()).items.sort(
														(a, b) => new Date(b.created).getTime() - new Date(a.created).getTime()
													);
													const lastProject = projects[0];
													let url: string;

													if (lastProject) {
														url = `/o/${lastProject.id}`;
													} else {
														const newProject = await EditorService.createObot();
														url = `/o/${newProject.id}`;
													}

													navigateTo(url, true);
												} finally {
													loadingChat = false;
												}
											}}
										>
											<link.icon class="size-5" />
											{link.label}
											<ExternalLink class="size-3" />
										</button>
									{:else}
										<a
											href={link.href}
											class={twMerge(
												'sidebar-link',
												link.href && link.href === pathname && 'bg-surface2',
												!link.href && 'no-link'
											)}
										>
											<link.icon class="size-5" />
											{link.label}
										</a>
									{/if}
									{#if !version.current.authEnabled && tooltips[link.href as keyof typeof tooltips]}
										<InfoTooltip text={tooltips[link.href as keyof typeof tooltips]} />
									{/if}
								</div>
								{#if link.collapsible}
									<button
										class="px-2"
										onclick={() => (collapsed[link.label] = !collapsed[link.label])}
									>
										{#if collapsed[link.label]}
											<ChevronUp class="size-5" />
										{:else}
											<ChevronDown class="size-5" />
										{/if}
									</button>
								{/if}
							</div>
							{#if !collapsed[link.label || '']}
								<div in:slide={{ axis: 'y' }}>
									{#if onRenderSubContent}
										{@render onRenderSubContent(link.label)}
									{/if}
									{#if link.items}
										<div class="flex flex-col px-7 text-sm font-light">
											{#each link.items as item (item.href)}
												<div class="relative">
													<div
														class={twMerge(
															'bg-surface3 absolute top-1/2 left-0 h-full w-0.5 -translate-x-3 -translate-y-1/2',
															item.href === pathname && 'bg-blue-500'
														)}
													></div>
													{#if item.disabled}
														<div class="sidebar-link disabled">
															<item.icon class="size-4" />
															{item.label}
														</div>
													{:else}
														<a
															href={item.href}
															class={twMerge(
																'sidebar-link',
																item.href === pathname && 'bg-surface2'
															)}
														>
															<item.icon class="size-4" />
															{item.label}
														</a>
													{/if}
												</div>
											{/each}
										</div>
									{/if}
								</div>
							{/if}
						{/each}
					</div>
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

		<Render
			class={twMerge(
				'default-scrollbar-thin relative flex h-svh w-full grow flex-col overflow-y-auto',
				whiteBackground ? 'bg-white dark:bg-black' : 'bg-surface1 dark:bg-black'
			)}
			component={main?.component}
			as="main"
			{...main?.props}
		>
			<Navbar class={twMerge('dark:bg-gray-990 sticky top-0 left-0 z-30 w-full', classes?.navbar)}>
				{#snippet leftContent()}
					{#if !layout.sidebarOpen || hideSidebar}
						<BetaLogo />
					{/if}
				{/snippet}
				{#snippet centerContent()}
					{#if layout.sidebarOpen && !hideSidebar}
						<div class="mx-8 flex w-full items-center gap-2" class:ml-4={showBackButton}>
							{@render layoutHeaderContent()}
						</div>
					{/if}
				{/snippet}
				{#snippet rightContent()}
					{#if rightNavActions && layout.sidebarOpen && !hideSidebar}
						{@render rightNavActions()}
					{/if}
				{/snippet}
			</Navbar>

			<div
				class={twMerge(
					'flex flex-1 flex-col items-center justify-center p-4 md:px-8',
					classes?.container
				)}
			>
				<div
					class={twMerge(
						'flex h-full w-full max-w-(--breakpoint-xl) flex-col',
						classes?.childrenContainer ?? ''
					)}
				>
					{#if isAdminRoute && !excludeConfigureBanner.includes(pathname)}
						<ConfigureBanner />
					{/if}
					{#if !layout.sidebarOpen || hideSidebar}
						<div class="flex w-full items-center justify-between gap-2">
							{@render layoutHeaderContent()}
							<div class="flex flex-shrink-0 items-center gap-2">
								{#if rightNavActions}
									{@render rightNavActions()}
								{/if}
							</div>
						</div>
					{/if}
					{@render children()}
				</div>
			</div>
		</Render>
	</div>

	{#if !layout.sidebarOpen && !hideSidebar}
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

{#if isAdminRoute}
	<SetupSplashDialog />
{/if}

<PageLoading show={loadingChat} text="Loading chat..." />

{#snippet layoutHeaderContent()}
	{#if showBackButton}
		<button
			class="icon-button flex-shrink-0"
			onclick={() => {
				if (onBackButtonClick) {
					onBackButtonClick();
				} else {
					history.back();
				}
			}}
		>
			<ChevronLeft class="size-6" />
		</button>
	{/if}
	{#if title}
		<h1 class="w-full text-xl font-semibold">{title}</h1>
	{/if}
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

		&.disabled {
			opacity: 0.5;
			cursor: default;
			&:hover {
				background-color: transparent;
			}
		}

		&.no-link {
			&:hover {
				background-color: transparent;
			}
		}
	}
</style>
