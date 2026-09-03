<script lang="ts" module>
	import { type Component, type Snippet } from 'svelte';

	type NavLink = {
		id: string;
		href?: string;
		icon?: Component;
		label: string;
		disabled?: boolean;
		collapsible?: boolean;
		items?: NavLink[];
		noteIcon?: Component;
		note?: Snippet;
		beta?: boolean;
		nodes?: NavLink[];
	};

	const NAV_COLLAPSED_KEY = '@obot/layout/nav-collapsed';

	const defaultNavCollapsed: Record<string, boolean> = {
		'agent-management': true,
		'mcp-server-management': true,
		'skills-management': true,
		'hosted-agent-management': true,
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

	type SidebarPane = 'default' | 'advanced';
	const sidebarScrollTopCache: Record<SidebarPane, number | null> = {
		default: null,
		advanced: null
	};
</script>

<script lang="ts">
	import { afterNavigate } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { columnResize } from '$lib/actions/resize';
	import Navbar from '$lib/components/Navbar.svelte';
	import {
		ADMIN_AGENT_DISABLED_MESSAGE,
		COMMUNITY_ENTITLEMENT,
		ENTERPRISE_ENTITLEMENT,
		USER_AGENT_DISABLED_MESSAGE
	} from '$lib/constants';
	import {
		initLayout as defaultInitLayout,
		getLayout as defaultGetLayout,
		type Layout as LayoutState
	} from '$lib/context/layout.svelte';
	import { localState } from '$lib/runes/localState.svelte';
	import {
		defaultModelAliases,
		license as licenseStore,
		profile,
		responsive,
		version,
		appNotification as appNotificationStore
	} from '$lib/stores';
	import { adminConfigStore } from '$lib/stores/adminConfig.svelte';
	import { isAgentEnabled, validateVersionUserLimit } from '$lib/utils';
	import AppNotificationBanner from './AppNotificationBanner.svelte';
	import DotDotDot from './DotDotDot.svelte';
	import InfoTooltip from './InfoTooltip.svelte';
	import SetupSplashDialog from './admin/SetupSplashDialog.svelte';
	import CommunitySignupBanner from './admin/license/CommunitySignupBanner.svelte';
	import LicenseViolationBanner from './admin/license/LicenseViolationBanner.svelte';
	import GuidePanel from './guides/GuidePanel.svelte';
	import Guide from './guides/Guides.svelte';
	import BetaLogo from './navbar/BetaLogo.svelte';
	import Profile from './navbar/Profile.svelte';
	import IconButton from './primitives/IconButton.svelte';
	import { Render } from './ui/render';
	import {
		ChevronDown,
		ChevronLeft,
		ChevronUp,
		Users,
		Bot,
		LayoutDashboard,
		PanelLeftOpen,
		PanelLeftClose,
		Menu,
		X,
		Logs,
		Settings2,
		MessageSquareText,
		ChevronRight,
		BotMessageSquare,
		LockOpen
	} from '@lucide/svelte';
	import { tick, untrack } from 'svelte';
	import { fade, slide, type TransitionConfig } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	let navCollapsed = $state({ ...navCollapsedCache });
	let animatingNavSectionId = $state<string | null>(null);

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
		titleContent?: Snippet;
		subtitle?: string;
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
		subtitle,
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
		alwaysShowHeaderTitle,
		titleContent
	}: Props = $props();
	let nav = $state<HTMLDivElement>();
	let sidebarScroll = $state<HTMLDivElement>();
	let pathname = $derived(page.url.pathname);

	function saveSidebarScroll() {
		if (!sidebarScroll) return;
		sidebarScrollTopCache.default = sidebarScroll.scrollTop;
	}

	async function restoreSidebarScroll() {
		await tick();
		if (!sidebarScroll) return;
		const scrollTop = sidebarScrollTopCache.default;
		if (scrollTop !== null) {
			sidebarScroll.scrollTop = scrollTop;
		}
	}

	// Whether the Obot Agent feature is enabled server-side. When false, agent entry
	// points are removed entirely (not just disabled). When the feature is enabled but
	// models aren't configured, agentLinkEnabled is false so links show as disabled.
	let agentsFeatureEnabled = $derived(version.current.agentsEnabled !== false);
	let hostedAgentsFeatureEnabled = $derived(version.current.hostedAgentsEnabled === true);
	let agentLinkEnabled = $derived(
		isAgentEnabled(defaultModelAliases.current) && agentsFeatureEnabled
	);

	let isBootStrapUser = $derived(profile.current.isBootstrapUser?.() ?? false);

	let hasLicenseEntitlementViolations = $derived(
		(version.current.licenseEntitlementViolations?.length ?? 0) > 0
	);
	const isNearUserLimit = $derived(validateVersionUserLimit(version.current));

	let routes = $derived([
		{
			id: 'mcp-dashboard',
			icon: LayoutDashboard,
			label: 'Dashboard',
			href: '/dashboard',
			collapsible: false
		},
		{
			id: 'ai-resources',
			icon: Bot,
			label: 'AI Resources',
			collapsible: true,
			items: [
				// {
				// 	id: 'vmcp',
				// 	label: 'vMCPs',
				// 	href: '/vmcps'
				// },
				{
					id: 'mcp-servers',
					label: 'MCP Servers',
					href: '/mcp-servers'
				},
				{
					id: 'skills',
					label: 'Skills',
					href: '/skills'
				},
				{
					id: 'models',
					label: 'Models',
					href: '/models'
				},
				...(profile.current.hasAdminAccess?.() && agentsFeatureEnabled
					? [
							{
								id: 'obot-agents',
								label: 'Agents',
								href: '/admin/agents',
								disabled: isBootStrapUser || !agentLinkEnabled
							}
						]
					: [])
			]
		},
		{
			id: 'operations',
			icon: Logs,
			label: 'Operations',
			collapsible: true,
			items: [
				{
					id: 'audit-logs',
					label: 'Audit Logs',
					href: '/audit-logs'
				},
				{
					id: 'usage',
					label: 'Usage',
					href: '/usage'
				},
				{
					id: 'inventory',
					label: 'Inventory',
					href: '/inventory',
					beta: true
				},
				...(profile.current.hasAdminAccess?.()
					? [
							{
								id: 'enforcement-events',
								label: 'Enforcement Events',
								href: '/admin/enforcement-events',
								beta: true
							}
						]
					: [])
			]
		},
		...(hostedAgentsFeatureEnabled
			? [
					{
						id: 'hosted-agents',
						label: 'Hosted Agents',
						icon: MessageSquareText,
						href: '/hosted-agents'
					}
				]
			: []),
		{
			id: 'identity-and-access',
			label: 'Identity & Access',
			icon: Users,
			href: '/identity-access'
		},
		...(profile.current.hasAdminAccess?.()
			? [
					{
						id: 'platform',
						label: 'Platform',
						icon: Settings2,
						href: '/admin/platform'
					}
				]
			: []),
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

	function collectBetaHrefs(links: NavLink[]): string[] {
		return links.flatMap((link) => [
			...(link.beta && link.href ? [link.href] : []),
			...(link.items ? collectBetaHrefs(link.items) : [])
		]);
	}

	let betaRoutes = $derived(collectBetaHrefs(routes));
	let isBetaRoute = $derived(
		betaRoutes.some((href) => pathname === href || pathname.startsWith(`${href}/`))
	);
	let logoVariant = $derived.by(() => {
		if (version.current.enterprise) return 'enterprise' as const;
		return 'community' as const;
	});
	$effect(() => {
		if (responsive.isMobile) {
			layout.sidebarOpen = false;
		}
	});

	afterNavigate(async ({ to }) => {
		if (to && routes.length > 0) {
			const currentPath = to.url.pathname;
			const parentNavLink = routes.find((link) =>
				link.items?.find(
					(item) =>
						item.href && (currentPath === item.href || currentPath.startsWith(`${item.href}/`))
				)
			);
			if (parentNavLink && isNavCollapsed(parentNavLink.id)) {
				toggleNavCollapsed(parentNavLink.id);
			}
		}

		await restoreSidebarScroll();
	});

	const isAgentRoute = $derived(pathname === '/agent' || pathname.startsWith('/agent/'));
	$effect(() => {
		const isAdminOrBootstrapUser =
			profile.current.loaded &&
			(profile.current.hasAdminAccess?.() || profile.current.isBootstrapUser?.());
		if (isAdminOrBootstrapUser) {
			adminConfigStore.initialize();
		}
	});

	untrack(() => (layoutContext?.initLayout ?? defaultInitLayout)());
	const layout = untrack(() => (layoutContext?.getLayout ?? defaultGetLayout)());

	type BannerDismissState = {
		dismissedAt?: string;
	};

	let bannerDismissed = localState<BannerDismissState | undefined>('@obot/banner', undefined, {
		parse: (ls) => {
			if (!ls) return undefined;
			try {
				const parsed = JSON.parse(ls) as string | BannerDismissState;
				if (typeof parsed === 'string') {
					return { dismissedAt: parsed } satisfies BannerDismissState;
				} else if (parsed && typeof parsed === 'object') {
					return {
						dismissedAt: typeof parsed.dismissedAt === 'string' ? parsed.dismissedAt : undefined
					} satisfies BannerDismissState;
				} else return undefined;
			} catch (_err) {
				return undefined;
			}
		}
	});

	function handleDismissBanner() {
		bannerDismissed.current = {
			dismissedAt: new Date().toISOString()
		} satisfies BannerDismissState;
	}

	const COMMUNITY_SIGNUP_BANNER_KEY = '@obot/dismiss-community-signup-banner';
	let communitySignupBannerDismissed = localState<BannerDismissState | undefined>(
		COMMUNITY_SIGNUP_BANNER_KEY,
		undefined,
		{
			parse: (value) => {
				if (!value) return undefined;
				try {
					const parsed = JSON.parse(value) as unknown;
					if (parsed && typeof parsed === 'object') {
						const dismissedAt = (parsed as BannerDismissState).dismissedAt;
						return {
							dismissedAt: typeof dismissedAt === 'string' ? dismissedAt : undefined
						} satisfies BannerDismissState;
					}
					return undefined;
				} catch {
					return undefined;
				}
			}
		}
	);

	function handleDismissCommunitySignupBanner() {
		communitySignupBannerDismissed.current = {
			dismissedAt: new Date().toISOString()
		} satisfies BannerDismissState;
	}

	function isCommunitySignupDismissedForCurrentProfile() {
		const dismissedAt = communitySignupBannerDismissed.current?.dismissedAt;
		const dismissedDate = dismissedAt ? new Date(dismissedAt) : undefined;
		const hasValidDismissedAt =
			dismissedDate !== undefined && !Number.isNaN(dismissedDate.getTime());
		if (!hasValidDismissedAt) return false;

		const profileCreatedMs = profile.current.created
			? new Date(profile.current.created).getTime()
			: undefined;
		if (
			profileCreatedMs === undefined ||
			Number.isNaN(profileCreatedMs) ||
			profileCreatedMs < dismissedDate.getTime()
		) {
			return true;
		}

		return false;
	}

	const hasCommunityOrEnterpriseLicense = $derived.by(() => {
		if (version.current.enterprise || licenseStore.current.enterprise) return true;
		const entitlements = [
			...(licenseStore.current.entitlements ?? []),
			...(version.current.licenseEntitlements ?? [])
		];
		return (
			entitlements.includes(COMMUNITY_ENTITLEMENT) || entitlements.includes(ENTERPRISE_ENTITLEMENT)
		);
	});

	const canShowCommunitySignup = $derived.by(() => {
		if (!(profile.current.hasAdminAccess?.() || profile.current.isBootstrapUser?.())) return false;
		if (hasCommunityOrEnterpriseLicense) return false;
		if (!communitySignupBannerDismissed.isReady) return false;
		return !isCommunitySignupDismissedForCurrentProfile();
	});

	let showAppNotificationBanner = $derived.by(() => {
		if (isAgentRoute) return false;

		const appNotification = appNotificationStore.current;
		if (!appNotification?.banner?.enabled) return false;
		if (!appNotification.banner.dismissible) return true; // enabled & not dismissible, always show
		if (!bannerDismissed.isReady) return false;

		const dismissedAt = bannerDismissed.current?.dismissedAt;
		const dismissedDate = dismissedAt ? new Date(dismissedAt) : undefined;
		const hasValidDismissedAt =
			dismissedDate !== undefined && !Number.isNaN(dismissedDate.getTime());
		const wasBannerUpdatedAfterDismissal =
			appNotification?.updated &&
			hasValidDismissedAt &&
			dismissedDate <= new Date(appNotification.updated);
		return !!(
			!hasValidDismissedAt ||
			(wasBannerUpdatedAfterDismissal && appNotification.banner.resetDismissed)
		);
	});
</script>

<div class="flex min-h-dvh items-center">
	<div class="relative flex min-w-0 grow">
		{#if leftSidebar}
			{@render leftSidebar()}
		{:else if layout.sidebarOpen && !hideSidebar}
			<div
				class={twMerge(
					'bg-base-100 dark:bg-base-200 flex max-h-dvh w-full min-w-dvw shrink-0 flex-col md:w-1/6 md:max-w-xl md:min-w-80.5',
					classes?.sidebarRoot
				)}
				transition:slide={{ axis: 'x' }}
				bind:this={nav}
			>
				<div class="flex h-16 shrink-0 items-center justify-between px-2">
					<BetaLogo variant={logoVariant} />
					{#if responsive.isMobile}
						<IconButton
							tooltip={{ text: 'Close Menu', placement: 'left' }}
							onclick={() => (layout.sidebarOpen = false)}
						>
							<X class="size-6" />
						</IconButton>
					{/if}
				</div>

				<div
					bind:this={sidebarScroll}
					class={twMerge(
						'text-md scrollbar-default-thin flex max-h-[calc(100vh-64px)] grow flex-col gap-8 overflow-y-auto pr-3 pl-2 font-medium',
						classes?.sidebar
					)}
				>
					<div class="flex flex-col gap-0.5 h-full grow">
						<!-- {#each allRoutesA as link (link.id)}
							{@render navLink(link)}
						{/each} -->
						{#each routes as link (link.id)}
							{@render navLink(link)}
						{/each}
					</div>
				</div>

				{#if !responsive.isMobile}
					<div class="flex justify-end px-3 py-2">
						<IconButton
							tooltip={{ text: 'Close Sidebar' }}
							onclick={() => (layout.sidebarOpen = false)}
						>
							<PanelLeftClose class="size-6" />
						</IconButton>
					</div>
				{/if}
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
				{:else if hasLicenseEntitlementViolations || isNearUserLimit}
					<LicenseViolationBanner warnUserLimit={isNearUserLimit}>
						{#snippet fallback()}
							{#if showAppNotificationBanner}
								<AppNotificationBanner
									data={appNotificationStore.current?.banner}
									onDismiss={handleDismissBanner}
								/>
							{/if}
						{/snippet}
					</LicenseViolationBanner>
				{:else if showAppNotificationBanner}
					<AppNotificationBanner
						data={appNotificationStore.current?.banner}
						onDismiss={handleDismissBanner}
					/>
				{:else if canShowCommunitySignup}
					<CommunitySignupBanner onDismiss={handleDismissCommunitySignupBanner} />
				{/if}
				<Navbar
					class={twMerge('dark:bg-base-200 border-b border-base-300', classes?.navbar)}
					{hideProfileButton}
				>
					{#snippet leftContent()}
						{#if overrideLeftMenu}
							{@render overrideLeftMenu()}
						{:else if (!layout.sidebarOpen || hideSidebar) && !leftSidebar}
							<div class="flex items-center gap-1.5">
								{#if responsive.isMobile}
									<IconButton
										class="w-fit"
										tooltip={{ text: 'Open Menu', placement: 'right' }}
										onclick={() => (layout.sidebarOpen = true)}
									>
										<Menu class="size-6" />
									</IconButton>
								{/if}
								<BetaLogo variant={logoVariant} />
							</div>
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

	{#if !layout.sidebarOpen && !hideSidebar && !leftSidebar && !responsive.isMobile}
		<div class="fixed bottom-2 left-2 z-30" in:fade={{ delay: 300 }}>
			<IconButton onclick={() => (layout.sidebarOpen = true)} tooltip={{ text: 'Open Sidebar' }}>
				<PanelLeftOpen class="size-6" />
			</IconButton>
		</div>
	{/if}

	{#if !isBootStrapUser && !responsive.isMobile}
		<GuidePanel />
		<Guide />
	{/if}
</div>

<SetupSplashDialog />

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
	{#if title || titleContent}
		<div class="flex flex-col md:w-full">
			{#if subtitle}
				<span class="text-xs font-light text-muted-content">{subtitle}</span>
			{/if}
			<h1
				class={twMerge(
					'text-xl font-semibold flex items-center gap-2',
					!layout.sidebarOpen && classes?.noSidebarTitle
				)}
			>
				{#if titleContent}
					{@render titleContent()}
				{:else}
					{title}
					{#if isBetaRoute}
						<span class="badge badge-primary badge-sm font-medium uppercase">Beta</span>
					{/if}
				{/if}
			</h1>
		</div>
	{/if}
{/snippet}

{#snippet renderAgentDisabledNote()}
	{#if !agentLinkEnabled}
		<p class="mt-1 text-sm">
			{profile.current.isAdmin?.() ? ADMIN_AGENT_DISABLED_MESSAGE : USER_AGENT_DISABLED_MESSAGE}
		</p>
	{/if}
{/snippet}

{#snippet dotdotdot(link: NavLink, alt?: boolean)}
	{#if link.nodes}
		{@const isChildActive = link.nodes.some(
			(node) => node.href && (node.href === pathname || pathname.startsWith(`${node.href}/`))
		)}
		<DotDotDot
			class="flex w-full items-center justify-start"
			placement="right-start"
			classes={{ popover: 'z-60', menu: 'min-w-64' }}
		>
			{#snippet icon()}
				{#if alt}
					<div class="relative flex items-center gap-2 w-full" id={link.id}>
						<div
							class={twMerge(
								'bg-base-400 absolute top-1/2 left-0 h-full w-0.5 -translate-x-3.25 -translate-y-1/2',
								isChildActive && 'bg-primary'
							)}
						></div>
						<span
							class={twMerge(
								'flex items-center gap-1 sidebar-link text-md font-medium justify-between w-full',
								isChildActive && 'bg-base-300'
							)}
						>
							<span class="flex items-center gap-1">
								{#if link.icon}
									<link.icon class="size-5" />
								{/if}
								{link.label}
							</span>
							<ChevronRight class="size-5" />
						</span>
					</div>
				{:else}
					<span
						class={twMerge(
							'flex items-center gap-1 sidebar-link text-md font-medium justify-between w-full',
							isChildActive && 'bg-base-300'
						)}
					>
						<span class="flex items-center gap-1">
							{#if link.icon}
								<link.icon class="size-5" />
							{/if}
							{link.label}
						</span>
						<ChevronRight class="size-5" />
					</span>
				{/if}
			{/snippet}
			{#each link.nodes as node (node.id)}
				{@render navLink(node)}
			{/each}
		</DotDotDot>
	{:else if link.href}
		{@render rootLinkContent(link)}
	{/if}
{/snippet}

{#snippet navLink(link: NavLink)}
	{#if link.nodes}
		{@render dotdotdot(link)}
	{:else}
		<div class="flex">
			{#if link.collapsible && !link.href}
				<button
					class="flex w-full items-center"
					onclick={() => toggleNavCollapsed(link.id)}
					id={`sidebar-collapse-${link.id}`}
				>
					{@render rootLinkContent(link)}
					<div class="px-2">
						{#if isNavCollapsed(link.id)}
							<ChevronDown class="size-5" />
						{:else}
							<ChevronUp class="size-5" />
						{/if}
					</div>
				</button>
			{:else}
				<div class="flex w-full items-center" id={link.id}>
					{@render rootLinkContent(link)}
				</div>
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
					<div class="flex flex-col pl-7 text-sm font-light mb-2">
						{#each link.items as item (item.label)}
							{#if item.nodes}
								{@render dotdotdot(item, true)}
							{:else}
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
											{@render linkContent(item)}
										</div>
									{:else if item.href}
										<a
											id={`sidebar-link-${item.id}`}
											href={resolve(item.href as `/${string}`)}
											class={twMerge('sidebar-link', isActive && 'bg-base-300')}
											onclick={saveSidebarScroll}
										>
											{@render linkContent(item)}
										</a>
									{:else}
										<div class="sidebar-link disabled">
											{@render linkContent(item)}
										</div>
									{/if}
									{#if item.noteIcon && item.note}
										<InfoTooltip icon={item.noteIcon} interactive>
											{@render item.note()}
										</InfoTooltip>
									{/if}
								</div>
							{/if}
						{/each}
					</div>
				{/if}
			</div>
		{/if}
	{/if}
{/snippet}

{#snippet rootLinkContent(link: NavLink)}
	{@const isActive = link.href && (link.href === pathname || pathname.startsWith(`${link.href}/`))}
	{#if link.disabled}
		<div class="sidebar-link disabled">
			{@render linkContent(link)}
		</div>
	{:else if link.href}
		<a
			id={`sidebar-link-${link.id}`}
			href={resolve(link.href as `/${string}`)}
			class={twMerge('sidebar-link', isActive && 'bg-base-300')}
			onclick={saveSidebarScroll}
		>
			{@render linkContent(link)}
		</a>
	{:else}
		<div class="sidebar-link no-link">
			{@render linkContent(link)}
		</div>
	{/if}

	{#if link.noteIcon && link.note}
		<InfoTooltip icon={link.noteIcon} interactive>
			{@render link.note()}
		</InfoTooltip>
	{/if}
{/snippet}

{#snippet linkContent(link: NavLink)}
	{#if link.icon}
		<link.icon class="size-5" />
	{/if}
	{link.label}
	{#if link.beta}
		<span class="badge badge-primary badge-xs font-medium uppercase">Beta</span>
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
			width: fit-content;
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
