<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import ConfirmDeleteAccount from '$lib/components/ConfirmDeleteAccount.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import { m } from '$lib/i18n';
	import { UserService } from '$lib/services';
	import { profile, errors, version, userDeviceSettings } from '$lib/stores';
	import { clearProductAnalyticsConsentDeferral } from '$lib/stores/productTelemetryConsent.svelte';
	import { success } from '$lib/stores/success';
	import { goto } from '$lib/url';
	import { getUserRoleLabel } from '$lib/utils';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import { User } from '@lucide/svelte';

	interface Props {
		onClose?: () => void;
	}

	let { onClose }: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
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
				success.add(m.account_logged_out_other_sessions());
				toRevoke = false;
			}
		} catch (error) {
			console.error('Failed to logout all sessions:', error);
			errors.items.push(new Error(m.account_log_out_other_sessions_failed()));
		}
	}

	async function deleteAccount() {
		try {
			await UserService.deleteProfile();
			clearProductAnalyticsConsentDeferral();
			goto('/oauth2/sign_out?rd=/');
		} catch (error) {
			console.error('Failed to delete account:', error);
			errors.items.push(new Error(m.account_delete_failed()));
		} finally {
			toDelete = false;
		}
	}

	function handleDisplay24HourFormatToggle(checked: boolean) {
		userDeviceSettings.setTimeFormat(checked ? '24h' : '12h');
	}
</script>

<button
	class="dropdown-link"
	onclick={() => {
		dialog?.open();
	}}
>
	<User class="size-4" />
	{m.account_my_account()}
</button>

<ResponsiveDialog
	bind:this={dialog}
	title={m.account_my_account()}
	class="w-full max-w-lg"
	classes={{ content: 'p-6' }}
	{onClose}
>
	<img
		src={profile.current.iconURL}
		alt=""
		class="mx-auto mb-3 h-28 w-28 rounded-full object-cover"
	/>
	<div class="flex flex-row py-3">
		<div class="w-1/2 max-w-[150px]">{m.account_display_name()}</div>
		<div class="w-1/2 wrap-break-word">{profile.current.displayName}</div>
	</div>
	<hr />
	<div class="flex flex-row py-3">
		<div class="w-1/2 max-w-[150px]">{m.account_email()}</div>
		<div class="w-1/2 wrap-break-word">{profile.current.email}</div>
	</div>
	<hr />
	<div class="flex flex-row py-3">
		<div class="w-1/2 max-w-[150px]">{m.account_role()}</div>
		<div class="w-1/2 wrap-break-word">
			{getUserRoleLabel(profile.current.effectiveRole)}
		</div>
	</div>
	<hr />

	<div class="flex flex-row items-center justify-between py-3">
		<div class="flex flex-col gap-1">
			<p>{m.account_24h_format()}</p>
			<span class="text-sm font-light opacity-70">
				{m.account_24h_format_description()}
			</span>
		</div>
		<Toggle
			label=""
			checked={userDeviceSettings.timeFormat === '24h'}
			onChange={handleDisplay24HourFormatToggle}
		/>
	</div>
	<hr />

	<div class="mt-2 flex flex-col gap-4 py-3">
		{#if version.current.sessionStore === 'db'}
			<button
				class="w-full btn btn-error"
				onclick={(e) => {
					e.preventDefault();
					toRevoke = !toRevoke;
					dialog?.close();
				}}>{m.account_log_out_other_sessions()}</button
			>
		{/if}
		<button
			class="w-full btn btn-error"
			onclick={(e) => {
				e.preventDefault();
				toDelete = !toDelete;
				dialog?.close();
			}}>{m.account_delete_my_account()}</button
		>
	</div>
</ResponsiveDialog>

<Confirm
	show={toRevoke}
	msg={m.account_log_out_other_sessions_confirm()}
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
