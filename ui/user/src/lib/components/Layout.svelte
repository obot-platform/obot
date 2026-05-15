<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { columnResize } from '$lib/actions/resize';
	import Navbar from '$lib/components/Navbar.svelte';
	import { ADMIN_AGENT_DISABLED_MESSAGE, USER_AGENT_DISABLED_MESSAGE } from '$lib/constants';
	import {
		initLayout as defaultInitLayout,
		getLayout as defaultGetLayout,
		type Layout as LayoutState
	} from '$lib/context/layout.svelte';
	import Bots from '$lib/icons/Bots.svelte';
	import { Group } from '$lib/services';
	import { defaultModelAliases, profile, responsive, version } from '$lib/stores';
	import { adminConfigStore } from '$lib/stores/adminConfig.svelte';
	import { isAgentEnabled } from '$lib/utils';
	import InfoTooltip from './InfoTooltip.svelte';
	import Tour from './Tour.svelte';
	import ConfigureBanner from './admin/ConfigureBanner.svelte';
	import SetupSplashDialog from './admin/SetupSplashDialog.svelte';
	import BetaLogo from './navbar/BetaLogo.svelte';
	import Profile from './navbar/Profile.svelte';
	import IconButton from './primitives/IconButton.svelte';
	import { Render } from './ui/render';
	import {
		AlarmClock,
		Boxes,
		Captions,
		ChartBarDecreasing,
		ChevronDown,
		ChevronLeft,
		ChevronUp,
		Funnel,
		GlobeLock,
		KeyRound,
		LockKeyhole,
		Palette,
		RadioTower,
		Server,
		SquareLibrary,
		UserCog,
		Users,
		Group as GroupIcon,
		BotMessageSquare,
		Coins,
		PencilRuler,
		Vault,
		LockOpen,
		CircleQuestionMark,
		ShieldAlert,
		ShieldX,
		Bot,
		LayoutDashboard,
		Notebook,
		Laptop,
		ScanLine,
		MonitorCheck,
		PanelRightClose,
		PanelLeftOpen
	} from 'lucide-svelte';
	import { type Component, type Snippet, untrack } from 'svelte';
	import { fade, slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	type LayoutContext = {
		initLayout: () => void;
		getLayout: () => LayoutState;
	};

	type NavLink = {
		id: string;
		href?: string;
		icon: Component | typeof Server;
		label: string;
		disabled?: boolean;
		collapsible?: boolean;
		items?: NavLink[];
		noteIcon?: Component | typeof CircleQuestionMark;
		note?: Snippet;
	};

	interface Props {
		classes?: {
			container?: string;
			childrenContainer?: string;
			navbar?: string;
			collapsedSidebarHeaderContent?: string;
			sidebar?: string;
			sidebarRoot?: string;
			noSidebarTitle?: string;
		};
		children: Snippet;
		onRenderSubContent?: Snippet<[string]>;
		hideSidebar?: boolean;
		whiteBackground?: boolean;
		main?: { component: Component; props?: Record<string, unknown> };
		navLinks?: NavLink[];
		rightNavActions?: Snippet;
		rightMenu?: Snippet;
		leftMenu?: Snippet;
		title?: string;
		showBackButton?: boolean;
		onBackButtonClick?: () => void;
		leftSidebar?: Snippet;
		rightSidebar?: Snippet;
		mobileDock?: Snippet;
		banner?: Snippet;
		layoutContext?: LayoutContext;
		disableResize?: boolean;
		hideProfileButton?: boolean;
		alwaysShowHeaderTitle?: boolean;
	}

	const {
		classes,
		children,
		onRenderSubContent,
		hideSidebar,
		whiteBackground,
		main,
		rightNavActions,
		title,
		showBackButton,
		onBackButtonClick,
		leftSidebar,
		leftMenu: overrideLeftMenu,
		rightSidebar,
		rightMenu: overrideRightMenu,
		mobileDock,
		banner,
		layoutContext,
		disableResize,
		hideProfileButton,
		alwaysShowHeaderTitle
	}: Props = $props();
	let nav = $state<HTMLDivElement>();
	let collapsed = $state<Record<string, boolean>>({});
	let pathname = $derived(page.url.pathname);

	let agentLinkEnabled = $derived(isAgentEnabled(defaultModelAliases.current));

	let isBootStrapUser = $derived(profile.current.isBootstrapUser?.() ?? false);
	let isAtLeastPowerUserPlus = $derived(profile.current.groups.includes(Group.POWERUSER_PLUS));
	let isAtLeastPowerUser = $derived(profile.current.groups.includes(Group.POWERUSER));
	let chatLinks = $derived<NavLink[]>(
		version.current.nanobotIntegration
			? [
					{
						id: 'launch-agent-chat',
						href: '/agent',
						icon: BotMessageSquare,
						disabled: isBootStrapUser || !agentLinkEnabled,
						label: 'Launch Agent',
						collapsible: false,
						noteIcon: !agentLinkEnabled ? LockOpen : undefined,
						note: !agentLinkEnabled ? renderAgentDisabledNote : undefined
					}
				]
			: []
	);
	let navLinks = $derived<NavLink[]>(
		profile.current.hasAdminAccess?.()
			? [
					{
						id: 'mcp-dashboard',
						icon: LayoutDashboard,
						label: 'Dashboard',
						href: '/admin/dashboard'
					},
					{
						id: 'mcp-server-management',
						icon: RadioTower,
						label: 'MCP Management',
						collapsible: true,
						items: [
							{
								id: 'mcp-servers',
								icon: Server,
								href: '/admin/mcp-servers',
								label: 'MCP Servers',
								disabled: isBootStrapUser,
								collapsible: false
							},
							{
								id: 'mcp-registries',
								icon: SquareLibrary,
								href: '/admin/mcp-registries',
								label: 'MCP Registries',
								disabled: isBootStrapUser,
								collapsible: false
							},
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
							},
							version.current.engine === 'kubernetes'
								? {
										id: 'server-scheduling',
										href: '/admin/server-scheduling',
										icon: AlarmClock,
										label: 'Server Scheduling',
										collapsible: false
									}
								: undefined
						].filter(Boolean) as NavLink[]
					},
					{
						id: 'skills-management',
						icon: Notebook,
						label: 'Skills Management',
						collapsible: true,
						items: [
							{
								id: 'skills',
								href: '/admin/skills',
								icon: PencilRuler,
								label: 'Skills',
								collapsible: false
							},
							{
								id: 'skill-access-policies',
								href: '/admin/skill-access-policies',
								icon: Vault,
								label: 'Skill Access Policies',
								collapsible: false
							}
						]
					},
					{
						id: 'device-management',
						icon: Laptop,
						label: 'Device Management',
						collapsible: true,
						items: [
							{
								id: 'device-overview',
								href: '/admin/device-overview',
								icon: LayoutDashboard,
								label: 'Dashboard',
								disabled: isBootStrapUser,
								collapsible: false
							},
							{
								id: 'devices',
								href: '/admin/devices',
								icon: ScanLine,
								label: 'Devices',
								disabled: isBootStrapUser,
								collapsible: false
							},
							{
								id: 'device-skills',
								href: '/admin/device-skills',
								icon: PencilRuler,
								label: 'Device Skills',
								disabled: isBootStrapUser,
								collapsible: false
							},
							{
								id: 'device-mcps',
								href: '/admin/device-mcp-servers',
								icon: Server,
								label: 'Device MCP Servers',
								disabled: isBootStrapUser,
								collapsible: false
							},
							{
								id: 'device-clients',
								href: '/admin/device-clients',
								icon: MonitorCheck,
								label: 'Device Clients',
								disabled: isBootStrapUser,
								collapsible: false
							}
						]
					},
					{
						id: 'user-management',
						icon: Users,
						label: 'User Management',
						disabled: !version.current.authEnabled,
						collapsible: true,
						noteIcon: !version.current.authEnabled ? LockOpen : undefined,
						note: !version.current.authEnabled ? renderAuthDisabledNote : undefined,
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
								id: 'groups',
								href: '/admin/groups',
								icon: GroupIcon,
								label: 'Groups',
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
							},
							{
								id: 'api-keys',
								href: '/admin/api-keys',
								icon: KeyRound,
								label: 'API Keys',
								disabled: !version.current.authEnabled,
								collapsible: false
							}
						]
					},

					{
						id: 'agent-management',
						icon: Bot,
						label: 'Obot Agent Management',
						collapsible: true,
						items: [
							{
								id: 'tokens',
								href: '/admin/token-usage',
								icon: Coins,
								label: 'Token Usage',
								disabled: isBootStrapUser,
								collapsible: false
							},
							{
								id: 'model-providers',
								href: '/admin/model-providers',
								icon: Boxes,
								label: 'Model Providers',
								collapsible: false
							},

							{
								id: 'model-access-policies',
								href: '/admin/model-access-policies',
								icon: LockKeyhole,
								label: 'Model Access Policies',
								collapsible: false
							},
							...(version.current.messagePoliciesEnabled
								? [
										{
											id: 'message-policies',
											href: '/admin/message-policies',
											icon: ShieldAlert,
											label: 'Message Policies',
											collapsible: false
										},
										{
											id: 'policy-violations',
											href: '/admin/policy-violations',
											icon: ShieldX,
											label: 'Message Policy Violations',
											collapsible: false
										}
									]
								: []),
							...(version.current.nanobotIntegration
								? [
										{
											id: 'admin-agents',
											href: '/admin/agents',
											icon: Bots,
											label: 'Agents',
											collapsible: false,
											disabled: isBootStrapUser || !agentLinkEnabled
										},
										{
											id: 'launch-agent-chat',
											href: '/agent',
											icon: BotMessageSquare,
											label: 'Launch Agent',
											disabled: isBootStrapUser || !agentLinkEnabled,
											collapsible: false,
											noteIcon: !agentLinkEnabled ? LockOpen : undefined,
											note: !agentLinkEnabled ? renderAgentDisabledNote : undefined
										}
									]
								: [])
						]
					},
					{
						id: 'app-preferences',
						href: '/admin/app-preferences',
						icon: Palette,
						label: 'Branding',
						disabled: false,
						collapsible: false
					}
				]
			: isAtLeastPowerUser
				? [
						{
							id: 'mcp-server-management',
							icon: RadioTower,
							label: 'MCP Management',
							collapsible: false,
							disabled: false,
							items: [
								{
									id: 'mcp-servers',
									href: '/mcp-servers',
									icon: Server,
									label: 'MCP Servers',
									disabled: false,
									collapsible: false
								},
								...(isAtLeastPowerUserPlus
									? [
											{
												id: 'mcp-registries',
												href: '/mcp-registries',
												icon: GlobeLock,
												label: 'MCP Registries',
												disabled: false,
												collapsible: false
											}
										]
									: []),
								{
									id: 'audit-logs',
									href: '/audit-logs',
									icon: Captions,
									label: 'Audit Logs',
									disabled: false,
									collapsible: false
								},
								{
									id: 'usage',
									href: '/usage',
									icon: ChartBarDecreasing,
									label: 'Usage',
									disabled: false,
									collapsible: false
								}
							]
						},
						...chatLinks
					]
				: [
						{
							id: 'mcp-server-management',
							icon: RadioTower,
							label: 'MCP Management',
							collapsible: false,
							disabled: false,
							items: [
								{
									id: 'mcp-servers',
									href: '/mcp-servers',
									icon: Server,
									label: 'MCP Servers',
									disabled: false,
									collapsible: false
								},
								{
									id: 'audit-logs',
									href: '/audit-logs',
									icon: Captions,
									label: 'Audit Logs',
									disabled: false,
									collapsible: false
								},
								{
									id: 'usage',
									href: '/usage',
									icon: ChartBarDecreasing,
									label: 'Usage',
									disabled: false,
									collapsible: false
								}
							]
						},
						...chatLinks
					]
	);

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
			(profile.current.hasAdminAccess?.() || profile.current.isBootstrapUser?.());
		if (isAdminOrBootstrapUser && isAdminRoute) {
			adminConfigStore.initialize();
			if (collapsed['agent-management'] === undefined) {
				collapsed['agent-management'] = true;
			}
		}
	});

	untrack(() => (layoutContext?.initLayout ?? defaultInitLayout)());
	const layout = untrack(() => (layoutContext?.getLayout ?? defaultGetLayout)());
</script>

<div class="flex min-h-dvh flex-col items-center">
	<div class="relative flex w-full grow">
		{#if leftSidebar}
			{@render leftSidebar()}
		{:else if layout.sidebarOpen && !hideSidebar}
			<div
				class={twMerge(
					'bg-base-100 dark:bg-base-200 flex max-h-dvh w-full min-w-dvw shrink-0 flex-col md:w-1/6 md:max-w-xl md:min-w-[310px]',
					classes?.sidebarRoot
				)}
				transition:slide={{ axis: 'x' }}
				bind:this={nav}
			>
				<div class="flex h-16 shrink-0 items-center px-2">
					<BetaLogo enterprise={version.current.enterprise} />
				</div>

				<div
					class={twMerge(
						'text-md scrollbar-default-thin flex max-h-[calc(100vh-64px)] grow flex-col gap-8 overflow-y-auto pr-3 pl-2 font-medium',
						classes?.sidebar
					)}
				>
					<div class="flex flex-col gap-1">
						{#each navLinks as link (link.id)}
							<div class="flex">
								<div class="flex w-full items-center" id={link.id}>
									{#if link.disabled}
										<div class="sidebar-link disabled">
											<link.icon class="size-5" />
											{link.label}
										</div>
									{:else if link.href}
										<a
											href={resolve(link.href as `/${string}`)}
											class={twMerge(
												'sidebar-link',
												link.href && link.href === pathname && 'bg-base-300'
											)}
										>
											<link.icon class="size-5" />
											{link.label}
										</a>
									{:else}
										<div class="sidebar-link no-link">
											<link.icon class="size-5" />
											{link.label}
										</div>
									{/if}

									{#if link.noteIcon && link.note}
										<InfoTooltip icon={link.noteIcon} interactive>
											{@render link.note()}
										</InfoTooltip>
									{/if}
								</div>
								{#if link.collapsible}
									<button class="px-2" onclick={() => (collapsed[link.id] = !collapsed[link.id])}>
										{#if collapsed[link.id]}
											<ChevronDown class="size-5" />
										{:else}
											<ChevronUp class="size-5" />
										{/if}
									</button>
								{/if}
							</div>
							{#if !collapsed[link.id]}
								<div in:slide={{ axis: 'y' }}>
									{#if onRenderSubContent}
										{@render onRenderSubContent(link.label)}
									{/if}
									{#if link.items}
										<div class="flex flex-col px-7 text-sm font-light">
											{#each link.items as item (item.href)}
												<div class="relative flex items-center gap-2" id={item.id}>
													<div
														class={twMerge(
															'bg-base-400 absolute top-1/2 left-0 h-full w-0.5 -translate-x-3 -translate-y-1/2',
															item.href === pathname && 'bg-primary'
														)}
													></div>
													{#if item.disabled}
														<div class="sidebar-link disabled">
															<div class="flex items-center gap-1 opacity-50">
																<item.icon class="size-4" />
																{item.label}
															</div>
														</div>
													{:else if item.href}
														<a
															href={resolve(item.href as `/${string}`)}
															class={twMerge(
																'sidebar-link',
																item.href === pathname && 'bg-base-300'
															)}
														>
															<item.icon class="size-4" />
															{item.label}
														</a>
													{:else}
														<div class="sidebar-link disabled">
															<item.icon class="size-4" />
															{item.label}
														</div>
													{/if}
													{#if item.noteIcon && item.note}
														<InfoTooltip icon={item.noteIcon} interactive>
															{@render item.note()}
														</InfoTooltip>
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
					<IconButton
						tooltip={{ text: 'Close Sidebar' }}
						onclick={() => (layout.sidebarOpen = false)}
					>
						<PanelRightClose class="size-6" />
					</IconButton>
				</div>
			</div>
			{#if !responsive.isMobile && !disableResize}
				<div
					role="none"
					class="h-inherit border-r-base-300 dark:border-r-base-300 relative -ml-3 w-3 cursor-col-resize border-r"
					use:columnResize={{ column: nav }}
				></div>
			{/if}
		{/if}

		<Render
			class={twMerge(
				'default-scrollbar-thin relative flex h-svh w-full min-w-0 grow flex-col overflow-y-auto',
				whiteBackground ? 'bg-base-100' : 'bg-base-200 dark:bg-base-100'
			)}
			component={main?.component}
			as="main"
			{...main?.props}
		>
			{#if banner}
				{@render banner()}
			{/if}
			<Navbar
				class={twMerge('dark:bg-base-100 sticky top-0 left-0 z-50 w-full', classes?.navbar)}
				{hideProfileButton}
			>
				{#snippet leftContent()}
					{#if overrideLeftMenu}
						{@render overrideLeftMenu()}
					{:else if (!layout.sidebarOpen || hideSidebar) && !leftSidebar}
						<BetaLogo />
					{/if}
				{/snippet}
				{#snippet centerContent()}
					{#if (layout.sidebarOpen && !hideSidebar) || alwaysShowHeaderTitle}
						<div
							class={twMerge(
								'flex w-full items-center gap-2',
								showBackButton ? 'md:ml-4' : 'md:mx-6'
							)}
						>
							{@render layoutHeaderContent()}
						</div>
					{/if}
				{/snippet}
				{#snippet rightContent()}
					{#if rightNavActions && layout.sidebarOpen && !hideSidebar}
						{@render rightNavActions()}
					{/if}
				{/snippet}
				{#snippet rightMenu()}
					{#if overrideRightMenu}
						{@render overrideRightMenu()}
					{:else if !hideProfileButton}
						<div class="flex h-16 shrink-0 items-center">
							<Profile />
						</div>
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
					{#if (!layout.sidebarOpen || hideSidebar) && !alwaysShowHeaderTitle}
						<div
							class={twMerge(
								'flex w-full items-center justify-between gap-2 pb-4',
								classes?.collapsedSidebarHeaderContent
							)}
						>
							{@render layoutHeaderContent()}
							<div class="flex shrink-0 items-center gap-2">
								{#if rightNavActions}
									{@render rightNavActions()}
								{/if}
							</div>
						</div>
					{/if}
					{@render children()}
				</div>
			</div>

			{#if mobileDock}
				{@render mobileDock()}
			{/if}
		</Render>

		{#if rightSidebar}
			{@render rightSidebar()}
		{/if}
	</div>

	{#if !layout.sidebarOpen && !hideSidebar && !leftSidebar}
		<div class="absolute bottom-2 left-2 z-30" in:fade={{ delay: 300 }}>
			<IconButton onclick={() => (layout.sidebarOpen = true)} tooltip={{ text: 'Open Sidebar' }}>
				<PanelLeftOpen class="size-6" />
			</IconButton>
		</div>
	{/if}
</div>

{#if isAdminRoute}
	<SetupSplashDialog />
{/if}

{#if !isBootStrapUser}
	<Tour />
{/if}

{#snippet layoutHeaderContent()}
	{#if showBackButton}
		<IconButton
			class="btn btn-square btn-ghost shrink-0"
			onclick={() => {
				if (onBackButtonClick) {
					onBackButtonClick();
				} else {
					history.back();
				}
			}}
		>
			<ChevronLeft class="size-6" />
		</IconButton>
	{/if}
	{#if title}
		<h1
			class={twMerge(
				'text-xl font-semibold md:w-full',
				!layout.sidebarOpen && classes?.noSidebarTitle
			)}
		>
			{title}
		</h1>
	{/if}
{/snippet}

{#snippet renderAuthDisabledNote()}
	{#if !version.current.authEnabled}
		<p class="mt-1 text-sm">
			Obot is running with authentication disabled. Click <a
				href="https://docs.obot.ai/installation/enabling-authentication/"
				rel="external noopener noreferrer"
				target="_blank"
				class="text-link">here</a
			> for details.
		</p>
	{/if}
{/snippet}

{#snippet renderAgentDisabledNote()}
	{#if !agentLinkEnabled}
		<p class="mt-1 text-sm">
			{profile.current.isAdmin?.() ? ADMIN_AGENT_DISABLED_MESSAGE : USER_AGENT_DISABLED_MESSAGE}
		</p>
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
			background-color: var(--color-base-400);
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
