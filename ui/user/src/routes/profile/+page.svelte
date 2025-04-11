<script lang="ts">
	import { darkMode } from '$lib/stores';
	import Profile from '$lib/components/navbar/Profile.svelte';
	import { ChatService } from '$lib/services';
	import { profile, responsive } from '$lib/stores';
	import { goto } from '$app/navigation';
	import Notifications from '$lib/components/Notifications.svelte';
	import Confirm from '$lib/components/Confirm.svelte';

	let toDelete = false;
</script>

<div class="flex h-full flex-col items-center">
	<div class="flex h-16 w-full items-center p-4 md:p-5">
		<div class="relative flex items-end">
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
		</div>
		<div class="grow"></div>
		<div class="flex items-center gap-1">
			{#if !responsive.isMobile}
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
			<Profile />
		</div>
	</div>

	<main
		class="colors-background relative flex w-full max-w-(--breakpoint-2xl) flex-col justify-center md:pb-12"
	>
		<div class="mt-8 flex w-full flex-col gap-8">
			<div class="flex w-full flex-col gap-4">
				<div
					class="sticky top-0 z-30 flex items-center gap-4 bg-white px-4 py-4 md:px-12 dark:bg-black"
				>
					<h3 class="flex flex-shrink-0 text-2xl font-semibold">My Account</h3>
				</div>
				<div class="mx-auto max-w-sm rounded-xl bg-white p-6 shadow-md">
					<img
						src={profile.current.iconURL}
						alt=""
						class="mx-auto h-28 w-28 rounded-full object-cover"
					/>
					<div class="flex flex-row py-4">
						<div class="w-1/2">Display Name:</div>
						<div class="w-1/2">{profile.current.getDisplayName?.()}</div>
					</div>
					<hr />
					<div class="flex flex-row py-2">
						<div class="w-1/2">Email:</div>
						<div class="w-1/2 break-words">{profile.current.email}</div>
					</div>
					<hr />
					<div class="flex flex-row py-2">
						<div class="w-1/2">Username:</div>
						<div class="w-1/2">{profile.current.username}</div>
					</div>
					<hr />
					<div class="flex flex-row py-2">
						<div class="w-1/2">Role:</div>
						<div class="w-1/2">{profile.current.role === 1 ? 'Admin' : 'User'}</div>
					</div>
					<hr />
					<div class="flex flex-row py-2">
						<div class="w-1/2">AuthProvider:</div>
						<div class="w-1/2">{profile.current.currentAuthProvider?.split('-')[0]}</div>
					</div>
					<hr />
					<div class="flex flex-row py-2">
						<button
							class="ml-auto rounded bg-red-600 px-4 py-2 font-medium text-white hover:bg-red-700"
							onclick={(e) => {
								e.preventDefault();
								toDelete = !toDelete;
							}}>Delete my account</button
						>
					</div>
				</div>
			</div>
		</div>
	</main>

	<Notifications />
</div>

<Confirm
	msg={`Delete your account?`}
	show={!!toDelete}
	onsuccess={async () => {
		if (!toDelete) return;
		try {
			await ChatService.deleteProfile();
			goto('/oauth2/sign_out?rd=/');
		} finally {
			toDelete = false;
		}
	}}
	oncancel={() => (toDelete = false)}
/>

<svelte:head>
	<title>Obot | My Account</title>
</svelte:head>
