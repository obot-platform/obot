<script lang="ts">
	import ProfileIcon from '$lib/components/profile/ProfileIcon.svelte';
	import { profile, responsive, darkMode } from '$lib/stores';
	import Menu from '$lib/components/navbar/Menu.svelte';
	import {
		Book,
		LayoutDashboard,
		User,
		LogOut,
		Moon,
		Sun,
		BadgeInfo,
		X,
		Server
	} from 'lucide-svelte/icons';
	import { twMerge } from 'tailwind-merge';
	import { version } from '$lib/stores';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import { AdminService } from '$lib/services';
	import { BOOTSTRAP_USER_ID } from '$lib/constants';

	let versionDialog = $state<HTMLDialogElement>();

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
			await AdminService.bootstrapLogout();
			window.location.href = '/oauth2/sign_out?rd=/';
		} catch (err) {
			console.error(err);
		}
	}
</script>

<Menu
	title={profile.current.getDisplayName?.() || 'Anonymous'}
	slide={responsive.isMobile ? 'left' : undefined}
	fixed={responsive.isMobile}
	classes={{
		dialog: twMerge(
			'p-0 md:w-fit overflow-hidden',
			responsive.isMobile &&
				'rounded-none h-[calc(100vh-64px)] left-0 top-[64px] !rounded-none w-full h-full'
		)
	}}
>
	{#snippet icon()}
		<ProfileIcon />
	{/snippet}
	{#snippet header()}
		<div class="flex w-full items-center justify-between gap-8 p-4">
			<div class="flex items-center gap-3">
				<ProfileIcon class="size-12" />
				<div class="flex grow flex-col">
					<span>
						{profile.current.getDisplayName?.() || 'Anonymous'}
					</span>
					<span class="text-sm text-gray-500">
						{profile.current.role === 1 ? 'Admin' : 'User'}
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
	{/snippet}
	{#snippet body()}
		<div class="flex flex-col gap-2 px-2 pb-4">
			{#if profile.current.role === 1}
				<a href="/admin/" rel="external" role="menuitem" class="link">
					<LayoutDashboard class="size-4" />Admin Dashboard
				</a>
				<a href="/mcp-servers" rel="external" role="menuitem" class="link">
					<Server class="size-4" />My MCP Servers
				</a>
			{/if}
			{#if responsive.isMobile}
				<a href="https://docs.obot.ai" rel="external" target="_blank" class="link"
					><Book class="size-4" />Docs</a
				>
			{/if}
			{#if profile.current.email}
				<a href="/profile" rel="external" role="menuitem" class="link"
					><User class="size-4" /> My Account</a
				>
				<a href="/oauth2/sign_out?rd=/" rel="external" role="menuitem" class="link"
					><LogOut class="size-4" /> Log out</a
				>
			{/if}
			{#if profile.current.username === BOOTSTRAP_USER_ID}
				<button class="link" onclick={handleBootstrapLogout}>
					<LogOut class="size-4" /> Log out
				</button>
			{/if}
		</div>
		{#if version.current.obot}
			<div class="flex justify-end p-2 text-xs text-gray-500">
				<div class="flex gap-2">
					<a href={getLink('obot', version.current.obot)} target="_blank" rel="external">
						{version.current.obot}
					</a>
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
	{/snippet}
</Menu>

<dialog bind:this={versionDialog} class="z-50 max-w-lg min-w-sm p-4">
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
</dialog>

<style lang="postcss">
	.link {
		font-size: var(--text-md);
		display: flex;
		width: 100%;
		align-items: center;
		gap: 0.5rem;
		border-radius: 0.5rem;
		padding: 0.5rem;
	}
	.link:hover {
		background-color: var(--surface3);
	}

	.dark-selected::after {
		transform: translateY(2rem);
		background-color: var(--surface1);
	}

	.light-selected::after {
		transform: translateY(0);
		background-color: white;
	}
</style>
