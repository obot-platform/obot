<script lang="ts">
	import Navbar from '$lib/components/Navbar.svelte';
	import { columnResize } from '$lib/actions/resize';
	import { profile, responsive, version } from '$lib/stores';
	import { initLayout, getLayout } from '$lib/context/layout.svelte';
	import type { Component, Snippet } from 'svelte';
	import { fade, slide } from 'svelte/transition';
	import {
		Boxes,
		Captions,
		ChartBarDecreasing,
		ChevronDown,
		ChevronUp,
		CircuitBoard,
		Cpu,
		Funnel,
		GlobeLock,
		LockKeyhole,
		MessageCircle,
		MessageCircleMore,
		Server,
		Settings,
		SidebarClose,
		SidebarOpen,
		UserCog,
		Users
	} from 'lucide-svelte';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { BOOTSTRAP_USER_ID } from '$lib/constants';
	import { twMerge } from 'tailwind-merge';
	import { afterNavigate } from '$app/navigation';
	import BetaLogo from './navbar/BetaLogo.svelte';
	import ConfigureBanner from './admin/ConfigureBanner.svelte';
	import InfoTooltip from './InfoTooltip.svelte';
	import { Render } from './ui/render';
	import { Group } from '$lib/services';

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
		};
		children: Snippet;
		showUserLinks?: boolean;
		onRenderSubContent?: Snippet<[string]>;
		hideSidebar?: boolean;
		whiteBackground?: boolean;
		main?: { component: Component; props?: Record<string, unknown> };
		navLinks?: NavLink[];
	}

	const {
		classes,
		children,
		showUserLinks,
		navLinks: overrideNavLinks,
		onRenderSubContent,
		hideSidebar,
		whiteBackground,
		main
	}: Props = $props();
	let nav = $state<HTMLDivElement>();
	let collapsed = $state<Record<string, boolean>>({});
	let pathname = $state('');

	let isBootStrapUser = $derived(profile.current.username === BOOTSTRAP_USER_ID);
	let navLinks = $derived<NavLink[]>(
		profile.current.hasAdminAccess?.() && !showUserLinks
			? [
					{
						id: 'mcp-server-management',
						icon: Server,
						label: 'MCP Server Management',
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
							{
								id: 'access-control',
								href: '/admin/access-control',
								icon: GlobeLock,
								label: 'Access Control',
								disabled: isBootStrapUser,
								collapsible: false
							}
						]
					},
					{
						id: 'obot-chat',
						icon: MessageCircle,
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
			: (overrideNavLinks ?? [
					{
						id: 'mcp-publisher',
						href: '/mcp-publisher',
						icon: Server,
						label: 'MCP Servers',
						disabled: false,
						collapsible: false
					},
					...(profile.current?.groups.includes(Group.POWERUSER_PLUS)
						? [
								{
									id: 'access-control',
									href: '/mcp-publisher/access-control',
									icon: GlobeLock,
									label: 'Access Control',
									disabled: false,
									collapsible: false
								}
							]
						: [])
				])
	);

	const tooltips = {
		'/admin/auth-providers': 'Enable authentication to access this page.'
	};

	afterNavigate(() => {
		pathname = window.location.pathname;
	});

	$effect(() => {
		if (responsive.isMobile) {
			layout.sidebarOpen = false;
		}
	});

	const excludeConfigureBanner = ['/admin/model-providers', '/admin/auth-providers'];

	initLayout();
	const layout = getLayout();
</script>

<div class="flex min-h-dvh flex-col items-center">
	<div class="relative flex w-full grow">
		{#if layout.sidebarOpen && !hideSidebar}
			<div
				class="dark:bg-gray-990 flex max-h-dvh w-dvh min-w-dvw flex-shrink-0 flex-col bg-white md:w-1/6 md:max-w-xl md:min-w-[300px]"
				transition:slide={{ axis: 'x' }}
				bind:this={nav}
			>
				<div class="flex h-16 flex-shrink-0 items-center px-2">
					<BetaLogo enterprise />
				</div>

				<div
					class="text-md scrollbar-default-thin flex max-h-[calc(100vh-64px)] grow flex-col gap-8 overflow-y-auto px-3 pt-8 pl-2 font-medium"
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
			<Navbar class="dark:bg-gray-990 sticky top-0 left-0 z-30 w-full">
				{#snippet leftContent()}
					{#if !layout.sidebarOpen || hideSidebar}
						<BetaLogo />
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
					{#if pathname.includes('/admin') && !excludeConfigureBanner.includes(pathname)}
						<ConfigureBanner />
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
			cursor: not-allowed;
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
