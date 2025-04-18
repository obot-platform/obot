<script lang="ts">
	import Profile from '$lib/components/navbar/Profile.svelte';
	import { ChatService } from '$lib/services';
	import { darkMode, profile, responsive, errors, version } from '$lib/stores';
	import { goto } from '$app/navigation';
	import Notifications from '$lib/components/Notifications.svelte';
	import ConfirmDeleteAccount from '$lib/components/ConfirmDeleteAccount.svelte';
	import { success } from '$lib/stores/success';
	import Confirm from '$lib/components/Confirm.svelte';
	import { ArrowLeft } from 'lucide-svelte/icons';

	let toDelete = $state(false);
	let toRevoke = $state(false);

	async function logoutAll() {
		try {
			const response = await fetch('/api/logout-all', {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json'
				}
			});
			if (response.ok) {
				success.add('Successfully logged out of all other sessions');
				toRevoke = false;
			}
		} catch (error) {
			console.error('Failed to logout all sessions:', error);
			errors.items.push(new Error('Failed to log out of other sessions'));
		}
	}

	async function deleteAccount() {
		try {
			await ChatService.deleteProfile();
			goto('/oauth2/sign_out?rd=/');
		} catch (error) {
			console.error('Failed to delete account:', error);
			errors.items.push(new Error('Failed to delete account'));
		} finally {
			toDelete = false;
		}
	}
</script>

<div class="flex h-full flex-col items-center">
	<div class="flex h-16 w-full items-center p-4 md:p-5">
		<a href="/catalog" class="relative flex items-end">
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
		</a>
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
				<div class="bg-surface1 mx-auto w-full max-w-4xl rounded-xl p-6 shadow-md">
					<a href="/home" class="mb-2 flex items-center gap-2 text-blue-500 hover:text-blue-600">
						<ArrowLeft class="size-4" />Go Home
					</a>
					<div class="flex flex-col-reverse sm:flex-row">
						<div class="w-full sm:w-1/2">
							<div class="flex flex-row py-3 2xl:flex-col">
								<div class="w-1/2 max-w-[150px] 2xl:w-full 2xl:py-2">Display Name:</div>
								<div class="w-1/2 break-words 2xl:w-full 2xl:py-2 2xl:text-xl">
									{profile.current.getDisplayName?.()}
								</div>
							</div>
							<hr />
							<div class="flex flex-row py-3 2xl:flex-col">
								<div class="w-1/2 max-w-[150px] 2xl:w-full 2xl:py-2">Email:</div>
								<div class="w-1/2 break-words 2xl:w-full 2xl:py-2 2xl:text-xl">
									{profile.current.email}
								</div>
							</div>
							<hr />
							<div class="flex flex-row py-3 2xl:flex-col">
								<div class="w-1/2 max-w-[150px] 2xl:w-full 2xl:py-2">Role:</div>
								<div class="w-1/2 break-words 2xl:w-full 2xl:py-2 2xl:text-xl">
									{profile.current.role === 1 ? 'Admin' : 'User'}
								</div>
							</div>
							<hr />
							<div class="mt-2 flex flex-col gap-4 py-3">
								{#if version.current.sessionStore === 'db'}
									<button
										class="w-full rounded-3xl border-2 border-red-600 px-4 py-2 font-medium text-red-600 hover:border-red-700 hover:text-red-700 2xl:rounded-4xl 2xl:py-3 2xl:text-lg"
										onclick={(e) => {
											e.preventDefault();
											toRevoke = !toRevoke;
										}}>Log out all other sessions</button
									>
								{/if}
								<button
									class="w-full rounded-3xl bg-red-600 px-4 py-2 font-medium text-white hover:bg-red-700 2xl:rounded-4xl 2xl:py-3 2xl:text-lg"
									onclick={(e) => {
										e.preventDefault();
										toDelete = !toDelete;
									}}>Delete my account</button
								>
							</div>
						</div>
						<div class="w-full sm:w-1/2">
							<img
								src={profile.current.iconURL}
								alt=""
								class="mx-auto mb-3 h-28 w-28 rounded-full object-cover sm:my-3 sm:h-48 sm:w-48"
							/>
						</div>
					</div>
				</div>
			</div>
		</div>
	</main>

	<Notifications />
</div>

<Confirm
	show={toRevoke}
	msg="Are you sure you want to log out of all other sessions? This will sign you out of all other devices and browsers, except for this one."
	onsuccess={logoutAll}
	oncancel={() => (toRevoke = false)}
/>

<ConfirmDeleteAccount
	msg="Are you sure you want to delete your account?"
	username={profile.current.username}
	show={!!toDelete}
	buttonText="Delete my account"
	onsuccess={deleteAccount}
	oncancel={() => (toDelete = false)}
/>

<svelte:head>
	<title>Obot | My Account</title>
</svelte:head>
