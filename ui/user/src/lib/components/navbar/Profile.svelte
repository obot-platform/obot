<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Menu from '$lib/components/navbar/Menu.svelte';
	import ProfileIcon from '$lib/components/profile/ProfileIcon.svelte';
	import { ADMIN_AGENT_DISABLED_MESSAGE, USER_AGENT_DISABLED_MESSAGE } from '$lib/constants';
	import { AdminService, ChatService, EditorService, NanobotService } from '$lib/services';
	import { profile, responsive, darkMode, errors, defaultModelAliases } from '$lib/stores';
	import { version } from '$lib/stores';
	import { goto } from '$lib/url';
	import { getUserRoleLabel, isAgentEnabled } from '$lib/utils';
	import Confirm from '../Confirm.svelte';
	import InfoTooltip from '../InfoTooltip.svelte';
	import PageLoading from '../PageLoading.svelte';
	import MyAccount from '../profile/MyAccount.svelte';
	import {
		Book,
		LogOut,
		Moon,
		Sun,
		BadgeInfo,
		X,
		MessageCircle,
		CircleFadingArrowUp,
		LayoutDashboard,
		KeyRound,
		BotMessageSquare,
		Power,
		LockOpen,
		HatGlasses
	} from 'lucide-svelte/icons';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		agentId?: string;
		projectId?: string;
		impersonating?: boolean;
	}

	let { agentId, projectId, impersonating }: Props = $props();

	let versionDialog = $state<HTMLDialogElement>();
	let loadingChat = $state(false);

	let inAdminRoute = $derived(page.url.pathname.includes('/admin'));
	let showChatLink = $derived(
		(!page.url.pathname.startsWith('/o') && !page.url.pathname.startsWith('/agent')) || inAdminRoute
	);
	let showApiKeysLink = $derived(
		!page.url.pathname.startsWith('/o') && !page.url.pathname.startsWith('/agent')
	);
	let showMcpManagement = $derived(
		['/o', '/profile', '/agent'].some((path) => page.url.pathname.startsWith(path))
	);
	let showRestartOption = $derived(
		page.url.pathname.startsWith('/agent') && !!agentId && !!projectId
	);

	let agentLinkEnabled = $derived(isAgentEnabled(defaultModelAliases.current));

	let showRestartAgentConfirm = $state(false);
	let restartingAgent = $state(false);

	let showUpgradeAvailable = $derived(
		version.current.authEnabled
			? profile.current.isAdmin?.()
				? version.current.upgradeAvailable
				: false
			: version.current.upgradeAvailable
	);

	function getLink(key: string, value: string | boolean) {
		if (typeof value !== 'string') return;

		const repoMap: Record<string, string> = {
			obot: 'https://github.com/obot-platform/obot'
		};

		const [, commit] = value.split('+');
		if (!repoMap[key] || !commit) return;

		return `${repoMap[key]}/commit/${commit}`;
	}

	async function handleBootstrapLogout() {
		try {
			localStorage.removeItem('seenSplashDialog');
			await AdminService.bootstrapLogout();
			window.location.href = `/oauth2/sign_out?rd=${profile.current.isBootstrapUser?.() ? '/admin' : '/'}`;
		} catch (err) {
			console.error(err);
		}
	}

	async function handleLogout() {
		try {
			localStorage.removeItem('seenSplashDialog');
			window.location.href = '/oauth2/sign_out?rd=/';
		} catch (err) {
			console.error(err);
		}
	}

	async function handleRestartAgent() {
		if (!agentId || !projectId) return;
		restartingAgent = true;
		try {
			await AdminService.restartK8sDeployment(`ms1${agentId}`);
			await NanobotService.launchProjectV2Agent(projectId, agentId);
			window.location.reload();
		} catch (error) {
			console.error('Failed to restart agent:', error);
			errors.append(error);
		} finally {
			restartingAgent = false;
			showRestartAgentConfirm = false;
		}
	}

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

<Menu
	title={profile.current.displayName || 'Anonymous'}
	slide={responsive.isMobile ? 'left' : undefined}
	fixed={responsive.isMobile}
	classes={{
		menu: twMerge(
			'p-0 md:w-fit overflow-hidden z-50',
			responsive.isMobile &&
				'rounded-none h-[calc(100vh-64px)] left-0 top-[64px] !rounded-none w-full h-full'
		)
	}}
>
	{#snippet icon()}
		<div class="relative flex-shrink-0">
			<ProfileIcon {impersonating} />
			{#if showUpgradeAvailable}
				<CircleFadingArrowUp
					class="text-primary bg-background absolute -right-0.5 -bottom-0.5 z-10 size-3 rounded-full"
				/>
			{/if}
		</div>
	{/snippet}
	{#snippet header()}
		<div class="flex w-full items-center justify-between gap-8 p-4 pb-2">
			<div class="flex items-center gap-3">
				<ProfileIcon class="size-12" {impersonating} />
				<div class="flex grow flex-col">
					<span>
						{profile.current.displayName || 'Anonymous'}
					</span>
					<span class="text-on-surface1 text-sm">
						{getUserRoleLabel(profile.current.effectiveRole)}
					</span>
				</div>
			</div>
			<button
				type="button"
				onclick={() => {
					darkMode.setDark(!darkMode.isDark);
				}}
				role="menuitem"
				class="after:content=[''] border-surface3 bg-surface2 dark:bg-surface3 relative cursor-pointer flex-col rounded-full border p-2 shadow-inner after:absolute after:top-1 after:left-1 after:z-0 after:size-7 after:rounded-full after:bg-transparent after:transition-all after:duration-200 dark:border-white/15"
				class:dark-selected={darkMode.isDark}
				class:light-selected={!darkMode.isDark}
			>
				<Sun class="relative z-10 mb-3 size-5" />
				<Moon class="relative z-10 size-5" />
			</button>
		</div>
		{#if impersonating}
			<div class="px-4">
				<div class="notification-info text-xs">
					<p class="flex items-center gap-1">
						<HatGlasses class="size-3" /> You are in impersonation mode.
					</p>
				</div>
			</div>
		{/if}
	{/snippet}
	{#snippet body()}
		<div class="flex flex-col gap-1 px-2 pb-4">
			{#if showRestartOption}
				<button
					class="dropdown-link"
					onclick={() => {
						showRestartAgentConfirm = true;
					}}
				>
					<Power class="size-4" /> Restart Agent
				</button>
			{/if}
			{#if responsive.isMobile}
				<a href="https://docs.obot.ai" rel="external" target="_blank" class="dropdown-link"
					><Book class="size-4" />Docs</a
				>
			{/if}
			{#if !impersonating}
				{#if profile.current.email && page.url.pathname !== '/profile'}
					<MyAccount />
				{/if}
				{#if showApiKeysLink}
					<a href={resolve('/keys')} role="menuitem" class="dropdown-link"
						><KeyRound class="size-4" />My API Keys</a
					>
				{/if}
				{#if profile.current.isBootstrapUser?.()}
					<button class="dropdown-link" onclick={handleBootstrapLogout}>
						<LogOut class="size-4" /> Log out
					</button>
				{:else}
					<button class="dropdown-link" onclick={handleLogout}>
						<LogOut class="size-4" /> Log out
					</button>
				{/if}
			{/if}
		</div>
		<div class="mt-2 p-2">
			{#if showChatLink && version.current.nanobotIntegration && !impersonating}
				<button
					class={twMerge(
						'dropdown-link',
						!agentLinkEnabled && 'cursor-default hover:bg-transparent dark:hover:bg-transparent'
					)}
					onclick={async (event) => {
						if (!agentLinkEnabled) return;
						navigateTo('/agent', event?.ctrlKey || event?.metaKey);
					}}
					aria-disabled={!agentLinkEnabled}
				>
					<span class={twMerge('flex items-center gap-1', !agentLinkEnabled && 'opacity-50')}>
						<BotMessageSquare class="size-4" /> Launch Agent
					</span>
					{#if !agentLinkEnabled}
						<InfoTooltip
							text={profile.current.isAdmin?.()
								? ADMIN_AGENT_DISABLED_MESSAGE
								: USER_AGENT_DISABLED_MESSAGE}
							icon={LockOpen}
						/>
					{/if}
				</button>
			{/if}
			{#if showChatLink && version.current.disableLegacyChat !== true && !impersonating}
				<button
					class="dropdown-link"
					class:mt-1={version.current.nanobotIntegration}
					onclick={async (event) => {
						const asNewTab = event?.ctrlKey || event?.metaKey;
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

							navigateTo(url, asNewTab);
						} finally {
							loadingChat = false;
						}
					}}
				>
					<MessageCircle class="size-4" />
					Launch Legacy Chat
				</button>
			{/if}
			{#if showMcpManagement && !impersonating}
				<a
					href={resolve(profile.current.hasAdminAccess?.() ? '/admin/mcp-servers' : '/mcp-servers')}
					rel="external"
					class="dropdown-link"
				>
					<LayoutDashboard class="size-4" /> MCP Platform
				</a>
			{/if}
			{#if version.current.obot}
				{#if showUpgradeAvailable}
					<div class="text-on-background flex items-center gap-1 p-1 text-[11px]">
						<CircleFadingArrowUp class="text-primary size-4 flex-shrink-0" />
						<p>
							Upgrade Available. <br /> Check out the
							<a
								rel="external"
								target="_blank"
								class="text-link"
								href="https://github.com/obot-platform/obot/releases/latest"
								>latest release notes.</a
							>
						</p>
					</div>
				{/if}
				<div class="text-on-surface1 flex justify-end p-2 text-xs">
					<div class="flex gap-2">
						{#if version.current.obot}
							{@const link = getLink('obot', version.current.obot)}
							{#if link}
								<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external github link -->
								<a href={link} target="_blank" rel="external">
									{version.current.obot}
								</a>
							{/if}
						{/if}
						<button
							use:tooltip={{ disablePortal: true, text: 'Versions' }}
							onclick={() => {
								versionDialog?.showModal();
							}}
						>
							<BadgeInfo class="size-3" />
						</button>
					</div>
				</div>
			{/if}
		</div>
	{/snippet}
</Menu>

<dialog bind:this={versionDialog} class="dialog">
	<div class="dialog-container relative z-50 max-w-lg min-w-sm p-4">
		<div class="absolute top-2 right-2">
			<button
				onclick={() => {
					versionDialog?.close();
				}}
				class="icon-button"
			>
				<X class="size-4" />
			</button>
		</div>
		<h4 class="mb-4 text-base font-semibold">Version Information</h4>
		<div class="flex flex-col gap-1 text-xs">
			{#each Object.entries(version.current) as [key, value] (key)}
				{@const canDisplay = typeof value === 'string' && value && key !== 'sessionStore'}
				{@const link = getLink(key, value)}
				{#if canDisplay}
					<div class="flex justify-between gap-8">
						<span class="font-semibold">{key.replace('github.com/', '')}:</span>
						{#if link}
							<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external version link -->
							<a href={link} target="_blank" rel="external">
								{value}
							</a>
						{:else}
							<span>{value}</span>
						{/if}
					</div>
				{/if}
			{/each}
		</div>
	</div>
	<form class="dialog-backdrop">
		<button
			type="button"
			aria-label="Close dialog"
			onclick={() => {
				versionDialog?.close();
			}}>close</button
		>
	</form>
</dialog>

<Confirm
	show={showRestartAgentConfirm}
	onsuccess={handleRestartAgent}
	oncancel={() => (showRestartAgentConfirm = false)}
	loading={restartingAgent}
	title="Restart Agent"
	msg="Are you sure you want to restart this agent?"
	type="info"
>
	{#snippet note()}
		This will restart the current agent with the latest available version. Are you sure you want to
		continue?
	{/snippet}
</Confirm>

<PageLoading show={loadingChat} text="Loading chat..." />

<style lang="postcss">
	.dark-selected::after {
		transform: translateY(2rem);
		background-color: var(--surface1);
	}

	.light-selected::after {
		transform: translateY(0);
		background-color: white;
	}
</style>
