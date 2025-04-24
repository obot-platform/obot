<script lang="ts">
	import ProfileIcon from '$lib/components/profile/ProfileIcon.svelte';
	import { profile, responsive, darkMode } from '$lib/stores';
	import Menu from '$lib/components/navbar/Menu.svelte';
	import { Book, LayoutDashboard, User, LogOut, Moon, Sun } from 'lucide-svelte/icons';
	import { twMerge } from 'tailwind-merge';
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
				<a href="/admin/" rel="external" role="menuitem" class="link"
					><LayoutDashboard class="size-4" />Admin Dashboard</a
				>
			{/if}
			{#if responsive.isMobile}
				<a href="https://docs.obot.ai" rel="external" target="_blank" class="link"
					><Book class="size-4" />Docs</a
				>
				<a href="https://discord.gg/9sSf4UyAMC" rel="external" target="_blank" class="link">
					{#if darkMode.isDark}
						<img
							src="/user/images/discord-mark/discord-mark-white.svg"
							alt="Discord"
							class="size-4"
						/>
					{:else}
						<img src="/user/images/discord-mark/discord-mark.svg" alt="Discord" class="size-4" />
					{/if}
					Discord
				</a>
				<a href="https://github.com/obot-platform/obot" rel="external" target="_blank" class="link">
					{#if darkMode.isDark}
						<img src="/user/images/github-mark/github-mark-white.svg" alt="GitHub" class="size-4" />
					{:else}
						<img src="/user/images/github-mark/github-mark.svg" alt="GitHub" class="size-4" />
					{/if}
					Github
				</a>
			{/if}
			{#if profile.current.email}
				<a href="/profile" rel="external" role="menuitem" class="link"
					><User class="size-4" /> My Account</a
				>
				<a href="/oauth2/sign_out?rd=/" rel="external" role="menuitem" class="link"
					><LogOut class="size-4" /> Log out</a
				>
			{/if}
		</div>
	{/snippet}
</Menu>

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
