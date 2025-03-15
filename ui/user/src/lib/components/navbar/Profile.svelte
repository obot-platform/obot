<script lang="ts">
	import ProfileIcon from '$lib/components/profile/ProfileIcon.svelte';
	import { profile, responsive } from '$lib/stores';
	import Menu from '$lib/components/navbar/Menu.svelte';
	import { Moon, Sun } from 'lucide-svelte/icons';
	import { darkMode } from '$lib/stores';
	import { twMerge } from 'tailwind-merge';
</script>

<Menu
	title={profile.current.getDisplayName?.() || 'Anonymous'}
	slide={responsive.isMobile ? 'left' : undefined}
	fixed={responsive.isMobile}
	classes={{
		dialog: twMerge(
			'px-4',
			responsive.isMobile &&
				'rounded-none h-[calc(100vh-64px)] p-4 left-0 top-[64px] !rounded-none w-full h-full'
		)
	}}
>
	{#snippet icon()}
		<ProfileIcon />
	{/snippet}
	{#snippet header()}
		<div class="flex w-full items-center justify-between">
			<span>
				{profile.current.getDisplayName?.() || 'Anonymous'}
			</span>
			<button
				type="button"
				onclick={() => {
					darkMode.setDark(!darkMode.isDark);
				}}
				role="menuitem"
				class="icon-button"
			>
				{#if darkMode.isDark}
					<Sun class="h-5 w-5" />
				{:else}
					<Moon class="h-5 w-5" />
				{/if}
			</button>
		</div>
	{/snippet}
	{#snippet body()}
		<div class="flex flex-col gap-2 py-2">
			{#if profile.current.role === 1}
				<a href="/admin/" rel="external" role="menuitem" class="icon-button" style="color: #f87171;"
					>Admin</a
				>
			{/if}
			{#if responsive.isMobile}
				<a href="https://docs.obot.ai" rel="external" target="_blank" class="icon-button">Docs</a>
				<a href="https://discord.gg/9sSf4UyAMC" rel="external" target="_blank" class="icon-button">
					{#if darkMode.isDark}
						<img src="/user/images/discord-mark/discord-mark-white.svg" alt="Discord" class="h-6" />
					{:else}
						<img src="/user/images/discord-mark/discord-mark.svg" alt="Discord" class="h-6" />
					{/if}
				</a>
				<a
					href="https://github.com/obot-platform/obot"
					rel="external"
					target="_blank"
					class="icon-button"
				>
					{#if darkMode.isDark}
						<img src="/user/images/github-mark/github-mark-white.svg" alt="GitHub" class="h-6" />
					{:else}
						<img src="/user/images/github-mark/github-mark.svg" alt="GitHub" class="h-6" />
					{/if}
				</a>
			{/if}
			{#if profile.current.email}
				<a href="/oauth2/sign_out?rd=/" rel="external" role="menuitem" class="icon-button"
					>Sign out</a
				>
			{/if}
		</div>
	{/snippet}
</Menu>
