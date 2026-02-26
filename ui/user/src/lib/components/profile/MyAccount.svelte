<script lang="ts">
	import { ChatService } from '$lib/services';
	import { profile, errors, version } from '$lib/stores';
	import { goto } from '$lib/url';
	import { getUserRoleLabel } from '$lib/utils';
	import ConfirmDeleteAccount from '$lib/components/ConfirmDeleteAccount.svelte';
	import { success } from '$lib/stores/success';
	import Confirm from '$lib/components/Confirm.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import { User } from 'lucide-svelte';

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let toDelete = $state(false);
	let toRevoke = $state(false);
	let savingPreferences = $state(false);
	let autonomousToolUseEnabled = $state(profile.current?.autonomousToolUseEnabled ?? false);

	// Sync with profile store when it updates
	$effect(() => {
		if (profile.current) {
			autonomousToolUseEnabled = profile.current.autonomousToolUseEnabled;
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

	async function handleAutonomousToolUseToggle(checked: boolean) {
		savingPreferences = true;
		try {
			const updatedProfile = await ChatService.patchProfile({
				autonomousToolUseEnabled: checked
			});
			autonomousToolUseEnabled = updatedProfile.autonomousToolUseEnabled;
			profile.initialize(updatedProfile);
		} catch (err) {
			console.error('Failed to update autonomous tool use setting:', err);
			autonomousToolUseEnabled = !checked;
		} finally {
			savingPreferences = false;
		}
	}
</script>

<button class="dropdown-link" onclick={() => dialog?.open()}>
	<User class="size-4" /> My Account
</button>

<ResponsiveDialog
	bind:this={dialog}
	title="My Account"
	class="w-full max-w-lg"
	classes={{ content: 'p-6' }}
>
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
			{getUserRoleLabel(profile.current.effectiveRole)}
		</div>
	</div>
	<hr />
	{#if !version.current.autonomousToolUseEnabled}
		<div class="flex flex-row items-center justify-between py-3">
			<div class="flex flex-col gap-1">
				<p>Allow Autonomous Tool Use</p>
				<span class="text-sm font-light opacity-70">
					When enabled, chat sessions can run tools automatically without asking for approval.
				</span>
			</div>
			<Toggle
				label=""
				checked={autonomousToolUseEnabled}
				disabled={savingPreferences}
				onChange={handleAutonomousToolUseToggle}
			/>
		</div>
		<hr />
	{/if}
	<div class="mt-2 flex flex-col gap-4 py-3">
		{#if version.current.sessionStore === 'db'}
			<button
				class="w-full rounded-3xl border-2 border-red-600 px-4 py-2 font-medium text-red-600 hover:border-red-700 hover:text-red-700"
				onclick={(e) => {
					e.preventDefault();
					toRevoke = !toRevoke;
					dialog?.close();
				}}>Log out all other sessions</button
			>
		{/if}
		<button
			class="w-full rounded-3xl bg-red-600 px-4 py-2 font-medium text-white hover:bg-red-700"
			onclick={(e) => {
				e.preventDefault();
				toDelete = !toDelete;
				dialog?.close();
			}}>Delete my account</button
		>
	</div>
</ResponsiveDialog>

<Confirm
	show={toRevoke}
	msg="Are you sure you want to log out of all other sessions? This will sign you out of all other devices and browsers, except for this one."
	onsuccess={logoutAll}
	oncancel={() => {
		toRevoke = false;
		dialog?.open();
	}}
/>

<ConfirmDeleteAccount
	username={profile.current.username}
	show={!!toDelete}
	onsuccess={deleteAccount}
	oncancel={() => {
		toDelete = false;
		dialog?.open();
	}}
/>

<svelte:head>
	<title>Obot | My Account</title>
</svelte:head>
