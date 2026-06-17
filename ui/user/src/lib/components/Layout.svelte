<script lang="ts" module>
	const NAV_COLLAPSED_KEY = '@obot/layout/nav-collapsed';

	const defaultNavCollapsed: Record<string, boolean> = {
		'agent-management': true,
		'mcp-server-management': true,
		'skills-management': true,
		'device-management': true,
		'user-management': true,
		'llm-gateway': true,
		advanced: true
	};

	function readNavCollapsedFromStorage(): Record<string, boolean> {
		if (typeof localStorage === 'undefined') return { ...defaultNavCollapsed };
		try {
			const local = localStorage.getItem(NAV_COLLAPSED_KEY);
			if (local) return { ...defaultNavCollapsed, ...JSON.parse(local) };
		} catch {
			// ignore invalid storage
		}
		return { ...defaultNavCollapsed };
	}

	let navCollapsedCache = readNavCollapsedFromStorage();
</script>

<script lang="ts">
	import { afterNavigate } from '$app/navigation';
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
	import LogoIcon from '$lib/icons/LogoIcon.svelte';
	import { localState } from '$lib/runes/localState.svelte';
	import { Group } from '$lib/services';
	import {
		accessibleModels,
		defaultModelAliases,
		profile,
		responsive,
		version,
		appNotifications as appNotificationsStore
	} from '$lib/stores';
	import { adminConfigStore } from '$lib/stores/adminConfig.svelte';
	import { isAgentEnabled } from '$lib/utils';
	import AppNotificationBanner from './AppNotificationBanner.svelte';
	import InfoTooltip from './InfoTooltip.svelte';
	import Tour from './Tour.svelte';
	import ConfigureBanner from './admin/ConfigureBanner.svelte';
	import SetupSplashDialog from './admin/SetupSplashDialog.svelte';
	import LicenseViolationBanner from './admin/license/LicenseViolationBanner.svelte';
	import BetaLogo from './navbar/BetaLogo.svelte';
	import Profile from './navbar/Profile.svelte';
	import IconButton from './primitives/IconButton.svelte';
	import { Render } from './ui/render';
	import {
		BrainCog,
		ChevronDown,
		ChevronLeft,
		ChevronUp,
		RadioTower,
		Server,
		Users,
		BotMessageSquare,
		PencilRuler,
		LockOpen,
		CircleQuestionMark,
		Bot,
		LayoutDashboard,
		Notebook,
		Laptop,
		PanelLeftOpen,
		Settings,
		PanelLeftClose,
		Brain,
		LayoutGrid
	} from 'lucide-svelte';
	import { type Component, type Snippet, untrack } from 'svelte';
	import { fade, slide, type TransitionConfig } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	let navCollapsed = $state({ ...navCollapsedCache });
	let showAdvancedPane = $state(untrack(() => isAdvancedPaneRoute(page.url.pathname)));
	let animatingNavSectionId = $state<string | null>(null);

	function isAdvancedPaneRoute(route: string): boolean {
		return (
			(route.includes('/admin') && route !== '/admin/dashboard') ||
			['/mcp-catalog', '/mcp-access-policies', '/audit-logs', '/usage'].some((p) =>
				route.startsWith(p)
			)
		);
	}

	function isNavCollapsed(id: string): boolean {
		return navCollapsed[id] ?? false;
	}

	function toggleNavCollapsed(id: string) {
		animatingNavSectionId = id;
		navCollapsed = { ...navCollapsed, [id]: !navCollapsed[id] };
		navCollapsedCache = navCollapsed;
		localStorage.setItem(NAV_COLLAPSED_KEY, JSON.stringify(navCollapsed));
	}

	function navSectionSlide(
		node: Element,
		{ id, axis = 'y' }: { id: string; axis?: 'x' | 'y' }
	): TransitionConfig {
		if (animatingNavSectionId !== id) {
			return { duration: 0 };
		}
		return slide(node, { axis, duration: 200 });
	}

	function clearNavSectionAnimation(id: string) {
		if (animatingNavSectionId === id) {
			animatingNavSectionId = null;
		}
	}

	type LayoutContext = {
		initLayout: () => void;
		getLayout: () => LayoutState;
	};

	type NavLink = {
		id: string;
		href?: string;
		icon?: Component | typeof Server;
		label: string;
		disabled?: boolean;
		collapsible?: boolean;
		items?: NavLink[];
		noteIcon?: Component | typeof CircleQuestionMark;
		note?: Snippet;
		beta?: boolean;
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
	let pathname = $derived(page.url.pathname);

	// Whether the Obot Agent feature is enabled server-side. When false, agent entry
	// points are removed entirely (not just disabled). When the feature is enabled but
	// models aren't configured, agentLinkEnabled is false so links show as disabled.
	let agentsFeatureEnabled = $derived(version.current.agentsEnabled !== false);
	let agentLinkEnabled = $derived(
		isAgentEnabled(defaultModelAliases.current) && agentsFeatureEnabled
	);

	let isBootStrapUser = $derived(profile.current.isBootstrapUser?.() ?? false);
	let isAtLeastPowerUser = $derived(profile.current.groups.includes(Group.POWERUSER));
	let isAtLeastPowerUserPlus = $derived(profile.current.groups.includes(Group.POWERUSER_PLUS));

	let hasAccessibleModels = $derived(accessibleModels.current.length > 0);

	let defaultLinks = $derived<NavLink[]>([
		...(profile.current.hasAdminAccess?.()
			? [
					{
						id: 'mcp-dashboard',
						icon: LayoutDashboard,
						label: 'Dashboard',
						href: '/admin/dashboard',
						collapsible: false
					}
				]
			: []),
		{
			id: 'mcp-servers',
			icon: Server,
			label: 'MCP Servers',
			href: '/mcp-servers'
		},
		{
			id: 'mcp-skills',
			icon: PencilRuler,
			label: 'Skills',
			href: '/skills'
		},
		...(hasAccessibleModels
			? [
					{
						id: 'llm-gateway-models',
						href: '/llm-gateway/models',
						icon: Brain,
						label: 'Models',
						collapsible: false
					}
				]
			: []),
		{
			id: 'devices',
			icon: Laptop,
			label: 'Devices',
			href: '/devices'
		},
		{
			id: 'install-cli',
			href: '/install-cli',
			icon: LogoIcon,
			label: 'Obot CLI',
			collapsible: false
		},
		...(agentsFeatureEnabled
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
			: [])
	]);

	let agentManagementLinks = $derived<NavLink[]>(
		agentsFeatureEnabled
			? [
					{
						id: 'agent-management',
						icon: Bot,
						label: 'Obot Agent Management',
						collapsible: true,
						items: [
							{
								id: 'admin-agents',
								href: '/admin/agents',
								icon: Bots,
								label: 'Agents',
								collapsible: false,
								disabled: isBootStrapUser || !agentLinkEnabled
							}
						]
					}
				]
			: []
	);

	let managementLinks = $derived<NavLink[]>(
		profile.current.hasAdminAccess?.()
			? [
					{
						id: 'mcp-server-management',
						icon: RadioTower,
						label: 'MCP Management',
						collapsible: true,
						items: [
							{
								id: 'mcp-catalog',
								href: '/admin/mcp-catalog',
								label: 'MCP Catalog',
								disabled: isBootStrapUser,
								collapsible: false
							},
							{
								id: 'mcp-access-policies',
								href: '/admin/mcp-access-policies',
								label: 'MCP Access Policies',
								disabled: isBootStrapUser,
								collapsible: false
							},
							{
								id: 'mcp-deployments',
								href: '/admin/mcp-deployments',
								label: 'MCP Deployments',
								collapsible: false
							},
							{
								id: 'audit-logs',
								href: '/admin/audit-logs',
								label: 'Audit Logs',
								disabled: isBootStrapUser,
								collapsible: false
							},
							{
								id: 'usage',
								href: '/admin/usage',
								label: 'Usage',
								disabled: isBootStrapUser,
								collapsible: false
							},
							{
								id: 'filters',
								href: '/admin/filters',
								label: 'Filters',
								disabled: isBootStrapUser
							},
							version.current.engine === 'kubernetes'
								? {
										id: 'server-scheduling',
										href: '/admin/server-scheduling',
										label: 'Server Scheduling',
										collapsible: false
									}
								: undefined,
							version.current.engine === 'kubernetes'
								? {
										id: 'image-pull-secrets',
										href: '/admin/image-pull-secrets',
										label: 'Image Pull Secrets',
										disabled: isBootStrapUser,
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
								label: 'Skill Sources',
								collapsible: false
							},
							{
								id: 'skill-access-policies',
								href: '/admin/skill-access-policies',
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
								label: 'Dashboard',
								disabled: isBootStrapUser,
								collapsible: false
							},
							{
								id: 'devices',
								href: '/admin/devices',
								label: 'Devices',
								disabled: isBootStrapUser,
								collapsible: false
							},
							{
								id: 'device-skills',
								href: '/admin/device-skills',
								label: 'Device Skills',
								disabled: isBootStrapUser,
								collapsible: false
							},
							{
								id: 'device-mcps',
								href: '/admin/device-mcp-servers',
								label: 'Device MCP Servers',
								disabled: isBootStrapUser,
								collapsible: false
							},
							{
								id: 'device-clients',
								href: '/admin/device-clients',
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
								label: 'Users',
								collapsible: false,
								disabled: !version.current.authEnabled
							},
							{
								id: 'groups',
								href: '/admin/groups',
								label: 'Groups',
								collapsible: false,
								disabled: !version.current.authEnabled
							},
							{
								id: 'user-roles',
								href: '/admin/user-roles',
								label: 'User Roles',
								collapsible: false,
								disabled: !version.current.authEnabled
							},
							{
								id: 'auth-providers',
								href: '/admin/auth-providers',
								label: 'Auth Providers',
								disabled: !version.current.authEnabled,
								collapsible: false
							},
							{
								id: 'api-keys',
								href: '/admin/api-keys',
								label: 'API Keys',
								disabled: !version.current.authEnabled,
								collapsible: false
							}
						]
					},
					{
						id: 'llm-gateway',
						icon: BrainCog,
						label: 'LLM Gateway',
						collapsible: true,
						items: [
							{
								id: 'tokens',
								href: '/admin/token-usage',
								label: 'Token Usage',
								disabled: isBootStrapUser,
								collapsible: false
							},
							{
								id: 'model-providers',
								href: '/admin/model-providers',
								label: 'Model Providers',
								collapsible: false
							},
							{
								id: 'model-access-policies',
								href: '/admin/model-access-policies',
								label: 'Model Access Policies',
								collapsible: false
							},
							...(version.current.messagePoliciesEnabled
								? [
										{
											id: 'message-policies',
											href: '/admin/message-policies',
											label: 'Message Policies',
											collapsible: false
										},
										{
											id: 'policy-violations',
											href: '/admin/policy-violations',
											label: 'Message Policy Violations',
											collapsible: false
										}
									]
								: [])
						]
					},
					...agentManagementLinks,
					{
						id: 'app-management',
						icon: LayoutGrid,
						label: 'App Management',
						collapsible: true,
						items: [
							{
								id: 'branding',
								href: '/admin/branding',
								label: 'Branding',
								disabled: false,
								collapsible: false
							},
							{
								id: 'app-notifications',
								href: '/admin/app-notifications',
								label: 'App Notifications',
								disabled: false,
								collapsible: false
							},
							{
								id: 'license',
								href: '/admin/license',
								label: 'License',
								disabled: false,
								collapsible: false
							}
						]
					}
				]
			: [
					{
						id: 'mcp-server-management',
						icon: RadioTower,
						label: 'MCP Management',
						collapsible: true,
						disabled: false,
						items: [
							...(isAtLeastPowerUser
								? [
										{
											id: 'mcp-catalog',
											href: '/mcp-catalog',
											label: 'MCP Catalog',
											disabled: false,
											collapsible: false
										},
										...(isAtLeastPowerUserPlus
											? [
													{
														id: 'mcp-access-policies',
														href: '/mcp-access-policies',
														label: 'MCP Access Policies',
														disabled: false,
														collapsible: false
													}
												]
											: [])
									]
								: []),
							{
								id: 'audit-logs',
								href: '/audit-logs',
								label: 'Audit Logs',
								disabled: false,
								collapsible: false
							},
							{
								id: 'usage',
								href: '/usage',
								label: 'Usage',
								disabled: false,
								collapsible: false
							}
						]
					}
				]
	);

	$effect(() => {
		if (responsive.isMobile) {
			layout.sidebarOpen = false;
		}
	});

	afterNavigate(({ to }) => {
		if (!to || managementLinks.length === 0) return;

		if (!isAdvancedPaneRoute(to.url.pathname)) {
			showAdvancedPane = false;
			return;
		}

		showAdvancedPane = true;
		const currentPath = to.url.pathname;
		const parentNavLink = managementLinks.find((link) =>
			link.items?.find(
				(item) =>
					item.href && (currentPath === item.href || currentPath.startsWith(`${item.href}/`))
			)
		);
		if (parentNavLink && isNavCollapsed(parentNavLink.id)) {
			toggleNavCollapsed(parentNavLink.id);
		}
	});

	const isAdminRoute = $derived(pathname.includes('/admin'));
	const excludeConfigureBanner = ['/admin/model-providers', '/admin/auth-providers'];
	$effect(() => {
		const isAdminOrBootstrapUser =
			profile.current.loaded &&
			(profile.current.hasAdminAccess?.() || profile.current.isBootstrapUser?.());
		if (isAdminOrBootstrapUser && isAdminRoute) {
			adminConfigStore.initialize();
		}
	});

	untrack(() => (layoutContext?.initLayout ?? defaultInitLayout)());
	const layout = untrack(() => (layoutContext?.getLayout ?? defaultGetLayout)());

	type BannerDismissState = {
		dismissedAt?: string;
		dismissedWithReset?: boolean;
	};

	let bannerDismissed = localState<BannerDismissState | undefined>('@obot/banner', undefined, {
		parse: (ls) => {
			if (!ls) return undefined;
			try {
				const parsed = JSON.parse(ls) as string | BannerDismissState;
				if (typeof parsed === 'string') {
					return { dismissedAt: parsed, dismissedWithReset: false } satisfies BannerDismissState;
				} else if (parsed && typeof parsed === 'object') {
					return {
						dismissedAt: typeof parsed.dismissedAt === 'string' ? parsed.dismissedAt : undefined,
						dismissedWithReset: parsed.dismissedWithReset === true
					} satisfies BannerDismissState;
				} else return undefined;
			} catch (_err) {
				return undefined;
			}
		}
	});

	function handleDismissBanner() {
		bannerDismissed.current = {
			dismissedAt: new Date().toISOString(),
			dismissedWithReset: appNotificationsStore.current?.banner?.resetDismissed === true
		} satisfies BannerDismissState;
	}

	let showAppNotificationBanner = $derived.by(() => {
		const appNotifications = appNotificationsStore.current;
		const dismissedAt = bannerDismissed.current?.dismissedAt;
		const wasDismissedAfterBannerUpdate =
			!!dismissedAt &&
			new Date(dismissedAt) >= new Date(appNotifications?.updated ?? new Date(0).toISOString());
		const shouldIgnorePriorDismissal =
			appNotifications?.banner?.resetDismissed === true &&
			bannerDismissed.current?.dismissedWithReset !== true;

		return !!(
			appNotifications?.banner?.enabled &&
			appNotifications?.updated &&
			(!wasDismissedAfterBannerUpdate || shouldIgnorePriorDismissal)
		);
	});
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
					{#if showAdvancedPane}
						<div class="flex flex-col gap-0.5 h-full" in:slide={{ axis: 'x', duration: 100 }}>
							<button class="sidebar-link" onclick={() => (showAdvancedPane = false)}>
								<ChevronLeft class="size-5 text-muted-content" />
								<span class="uppercase text-xs font-semibold tracking-wide text-muted-content">
									Back to App
								</span>
							</button>
							{#each managementLinks as link (link.id)}
								{@render navLink(link)}
							{/each}
						</div>
					{:else}
						<div class="flex flex-col gap-0.5 h-full">
							{#each defaultLinks as link (link.id)}
								{@render navLink(link)}
							{/each}

							<div class="flex grow"></div>

							{#if managementLinks.length > 0}
								<button class="sidebar-link" onclick={() => (showAdvancedPane = true)}>
									<Settings class="size-5 text-muted-content" />
									<span class="uppercase text-xs font-semibold tracking-wide text-muted-content">
										{profile.current.hasAdminAccess?.() ? 'Administration' : 'Advanced Settings'}
									</span>
								</button>
							{/if}
						</div>
					{/if}
				</div>

				<div class="flex justify-end px-3 py-2">
					<IconButton
						tooltip={{ text: 'Close Sidebar' }}
						onclick={() => (layout.sidebarOpen = false)}
					>
						<PanelLeftClose class="size-6" />
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
			<div class="sticky top-0 left-0 z-50 w-full">
				{#if banner}
					{@render banner()}
				{:else if (version.current.licenseEntitlementViolations?.length ?? 0) > 0}
					<LicenseViolationBanner />
				{:else if showAppNotificationBanner}
					<AppNotificationBanner
						data={appNotificationsStore.current?.banner}
						onDismiss={handleDismissBanner}
					/>
				{/if}
				<Navbar class={twMerge('dark:bg-base-100', classes?.navbar)} {hideProfileButton}>
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
			</div>

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
								'flex w-full items-center justify-between gap-2 pb-4 flex-wrap md:flex-nowrap',
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

{#snippet navLink(link: NavLink)}
	{@const isActive = link.href && (link.href === pathname || pathname.startsWith(`${link.href}/`))}
	<div class="flex">
		<div class="flex w-full items-center" id={link.id}>
			{#if link.disabled}
				<div class="sidebar-link disabled">
					{#if link.icon}
						<link.icon class="size-5" />
					{/if}
					{link.label}
				</div>
			{:else if link.href}
				<a
					href={resolve(link.href as `/${string}`)}
					class={twMerge('sidebar-link', isActive && 'bg-base-300')}
				>
					{#if link.icon}
						<link.icon class="size-5" />
					{/if}
					{link.label}
					{#if link.beta}
						<span class="badge badge-primary badge-xs">Beta</span>
					{/if}
				</a>
			{:else}
				<div class="sidebar-link no-link">
					{#if link.icon}
						<link.icon class="size-5" />
					{/if}
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
			<button class="px-2" onclick={() => toggleNavCollapsed(link.id)}>
				{#if isNavCollapsed(link.id)}
					<ChevronDown class="size-5" />
				{:else}
					<ChevronUp class="size-5" />
				{/if}
			</button>
		{/if}
	</div>
	{#if !isNavCollapsed(link.id)}
		<div
			in:navSectionSlide={{ id: link.id, axis: 'y' }}
			out:navSectionSlide={{ id: link.id, axis: 'y' }}
			onintroend={() => clearNavSectionAnimation(link.id)}
			onoutroend={() => clearNavSectionAnimation(link.id)}
		>
			{#if onRenderSubContent}
				{@render onRenderSubContent(link.label)}
			{/if}
			{#if link.items}
				<div class="flex flex-col px-7 text-sm font-light mb-2">
					{#each link.items as item (item.href)}
						{@const isActive =
							item.href && (item.href === pathname || pathname.startsWith(`${item.href}/`))}
						<div class="relative flex items-center gap-2" id={item.id}>
							<div
								class={twMerge(
									'bg-base-400 absolute top-1/2 left-0 h-full w-0.5 -translate-x-3 -translate-y-1/2',
									isActive && 'bg-primary'
								)}
							></div>
							{#if item.disabled}
								<div class="sidebar-link disabled">
									<div class="flex items-center gap-1 opacity-50">
										{#if item.icon}
											<item.icon class="size-4" />
										{/if}
										{item.label}
									</div>
								</div>
							{:else if item.href}
								<a
									href={resolve(item.href as `/${string}`)}
									class={twMerge('sidebar-link', isActive && 'bg-base-300')}
								>
									{#if item.icon}
										<item.icon class="size-4" />
									{/if}
									{item.label}
								</a>
							{:else}
								<div class="sidebar-link disabled">
									{#if item.icon}
										<item.icon class="size-4" />
									{/if}
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
