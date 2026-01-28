<script lang="ts">
	import { ChatService } from '$lib/services';
	import { profile, errors, version } from '$lib/stores';
	import { goto } from '$lib/url';
	import Notifications from '$lib/components/Notifications.svelte';
	import { getUserRoleLabel } from '$lib/utils';
	import ConfirmDeleteAccount from '$lib/components/ConfirmDeleteAccount.svelte';
	import { success } from '$lib/stores/success';
	import Confirm from '$lib/components/Confirm.svelte';
	import Navbar from '$lib/components/Navbar.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import { AlertTriangle } from 'lucide-svelte';

	let toDelete = $state(false);
	let toRevoke = $state(false);
	let savingPreferences = $state(false);
	let disableChatToolConfirm = $state(profile.current?.disableChatToolConfirm ?? false);

	// Sync with profile store when it updates
	$effect(() => {
		if (profile.current) {
			disableChatToolConfirm = profile.current.disableChatToolConfirm;
		}
	});

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

	async function handleDisableConfirmToggle(checked: boolean) {
		savingPreferences = true;
		try {
			const updatedProfile = await ChatService.patchProfile({
				disableChatToolConfirm: checked
			});
			disableChatToolConfirm = updatedProfile.disableChatToolConfirm;
			profile.initialize(updatedProfile);
		} catch (err) {
			console.error('Failed to update tool confirmation setting:', err);
			disableChatToolConfirm = !checked;
		} finally {
			savingPreferences = false;
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
				<div class="bg-background sticky top-0 z-30 flex items-center gap-4 px-4 py-4 md:px-12">
					<h3 class="flex flex-shrink-0 text-2xl font-semibold">My Account</h3>
				</div>
				<div class="bg-surface1 mx-auto w-full max-w-lg rounded-xl p-6 shadow-md">
					<img
						src={profile.current.iconURL}
						alt=""
						class="mx-auto mb-3 h-28 w-28 rounded-full object-cover"
					/>
					<div class="rounded-lg p-4">
						<div class="flex flex-row py-3">
							<div class="w-1/2 max-w-[150px]">Display Name:</div>
							<div class="w-1/2 break-words">{profile.current.displayName}</div>
						</div>
						<div class="border-surface3 border-t"></div>
						<div class="flex flex-row py-3">
							<div class="w-1/2 max-w-[150px]">Email:</div>
							<div class="w-1/2 break-words">{profile.current.email}</div>
						</div>
						<div class="border-surface3 border-t"></div>
						<div class="flex flex-row py-3">
							<div class="w-1/2 max-w-[150px]">Role:</div>
							<div class="w-1/2 break-words">
								{getUserRoleLabel(profile.current.effectiveRole)}
							</div>
						</div>
					</div>

					<!-- Danger Zone -->
					<div class="mt-4 rounded-lg border border-red-500 p-4">
						<div class="mb-4 flex items-center gap-2">
							<AlertTriangle class="size-5 text-red-500" />
							<h3 class="text-xl font-semibold">Danger Zone</h3>
						</div>

						<p class="mb-4 text-sm opacity-70">
							These actions can have significant consequences. Please proceed with caution.
						</p>

						<!-- Allow Autonomous Tool Use Toggle -->
						{#if !version.current.disableChatToolConfirm}
							<div
								class="border-surface3 mb-4 flex items-center justify-between gap-4 border-b pb-4"
							>
								<div class="flex flex-col gap-1">
									<p class="font-semibold">Allow Autonomous Tool Use</p>
									<span class="text-sm font-light opacity-70">
										When enabled, chat sessions can run tools automatically without asking for
										approval.
									</span>
								</div>
								<Toggle
									label=""
									checked={disableChatToolConfirm}
									disabled={savingPreferences}
									onChange={handleDisableConfirmToggle}
								/>
							</div>
						{/if}

						<!-- Log Out All Other Sessions -->
						{#if version.current.sessionStore === 'db'}
							<div class="border-surface3 mb-4 flex flex-col gap-2 border-b pb-4">
								<p class="font-semibold">Log Out All Other Sessions</p>
								<span class="text-sm font-light opacity-70">
									Sign out of all other devices and browsers, except for this one.
								</span>
								<button
									class="mt-2 w-full rounded-3xl border-2 border-red-600 px-4 py-2 font-medium text-red-600 hover:border-red-700 hover:text-red-700"
									onclick={(e) => {
										e.preventDefault();
										toRevoke = true;
									}}
								>
									Log out all other sessions
								</button>
							</div>
						{/if}

						<!-- Delete My Account -->
						<div class="flex flex-col gap-2">
							<p class="font-semibold">Delete My Account</p>
							<span class="text-sm font-light opacity-70">
								Permanently delete your account and all associated data. This action cannot be
								undone.
							</span>
							<button
								class="mt-2 w-full rounded-3xl bg-red-600 px-4 py-2 font-medium text-white hover:bg-red-700"
								onclick={(e) => {
									e.preventDefault();
									toDelete = true;
								}}
							>
								Delete my account
							</button>
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
