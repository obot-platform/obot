<script lang="ts">
	import { ChatService } from '$lib/services';
	import { profile, errors, version } from '$lib/stores';
	import { goto } from '$app/navigation';
	import Notifications from '$lib/components/Notifications.svelte';
	import { Group } from '$lib/services/admin/types';
	import ConfirmDeleteAccount from '$lib/components/ConfirmDeleteAccount.svelte';
	import { success } from '$lib/stores/success';
	import Confirm from '$lib/components/Confirm.svelte';
	import Navbar from '$lib/components/Navbar.svelte';

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
	<Navbar />

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
				<div class="bg-surface1 mx-auto w-full max-w-lg rounded-xl p-6 shadow-md">
					<img
						src={profile.current.iconURL}
						alt=""
						class="mx-auto mb-3 h-28 w-28 rounded-full object-cover"
					/>
					<div class="flex flex-row py-3">
						<div class="w-1/2 max-w-[150px]">Display Name:</div>
						<div class="w-1/2 break-words">{profile.current.displayName}</div>
					</div>
					<hr />
					<div class="flex flex-row py-3">
						<div class="w-1/2 max-w-[150px]">Email:</div>
						<div class="w-1/2 break-words">{profile.current.email}</div>
					</div>
					<hr />
					<div class="flex flex-row py-3">
						<div class="w-1/2 max-w-[150px]">Role:</div>
						<div class="w-1/2 break-words">
							{profile.current.groups.includes(Group.ADMIN) ? 'Admin' : 'User'}
						</div>
					</div>
					<hr />
					<div class="mt-2 flex flex-col gap-4 py-3">
						{#if version.current.sessionStore === 'db'}
							<button
								class="w-full rounded-3xl border-2 border-red-600 px-4 py-2 font-medium text-red-600 hover:border-red-700 hover:text-red-700"
								onclick={(e) => {
									e.preventDefault();
									toRevoke = !toRevoke;
								}}>Log out all other sessions</button
							>
						{/if}
						<button
							class="w-full rounded-3xl bg-red-600 px-4 py-2 font-medium text-white hover:bg-red-700"
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
