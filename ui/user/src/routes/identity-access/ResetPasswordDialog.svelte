<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import { LOCAL_AUTH_MIN_PASSWORD_LENGTH } from '$lib/constants';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService } from '$lib/services';

	interface Props {
		onSaved: () => Promise<void> | void;
	}

	let { onSaved }: Props = $props();

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let localAuthUserId = $state<string>();
	let email = $state('');
	let password = $state('');
	let requirePasswordChange = $state(true);
	let error = $state<string>();
	let saving = $state(false);

	export function open(user: { id: string; email: string }) {
		localAuthUserId = user.id;
		email = user.email;
		password = '';
		requirePasswordChange = true;
		error = undefined;
		dialog?.open();
	}

	function errorMessage(err: unknown) {
		if (!(err instanceof Error)) return m.identity_access_auth_providers_local_auth_save_failed();
		return (
			parseErrorContent(err).message || m.identity_access_auth_providers_local_auth_save_failed()
		);
	}

	async function save(e?: SubmitEvent) {
		e?.preventDefault();
		if (!localAuthUserId) return;
		if (!password || password.length < LOCAL_AUTH_MIN_PASSWORD_LENGTH) {
			error = !password
				? m.identity_access_auth_providers_local_auth_fill_password()
				: m.identity_access_auth_providers_local_auth_password_min({
						min: LOCAL_AUTH_MIN_PASSWORD_LENGTH
					});
			return;
		}

		saving = true;
		error = undefined;
		try {
			await AdminService.setLocalAuthUserPassword(localAuthUserId, password, requirePasswordChange);
		} catch (err) {
			error = errorMessage(err);
			return;
		} finally {
			saving = false;
		}

		dialog?.close();
		localAuthUserId = undefined;
		email = '';
		password = '';
		requirePasswordChange = true;
		await onSaved();
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	class="w-full overflow-visible p-4 md:max-w-xl"
	title={m.identity_access_auth_providers_local_auth_reset_password()}
	onClose={() => {
		localAuthUserId = undefined;
		error = undefined;
	}}
>
	{#if localAuthUserId}
		<form class="m-4 flex flex-col gap-4 text-sm font-light md:m-0" onsubmit={save}>
			<p class="text-muted-content">{email}</p>

			{#if error}
				<p class="text-error text-xs font-light" role="alert">{error}</p>
			{/if}

			<label class="flex flex-col gap-1" for="users-reset-password">
				{m.identity_access_auth_providers_local_auth_new_password()}
				<SensitiveInput
					name="users-reset-password"
					bind:value={password}
					autocomplete="new-password"
					minlength={LOCAL_AUTH_MIN_PASSWORD_LENGTH}
					required
					disabled={saving}
					error={!!error}
					data1pIgnore={false}
					oninput={() => (error = undefined)}
				/>
				<span class="text-muted-content pt-0.5 text-xs">
					{m.identity_access_auth_providers_local_auth_password_hint({
						min: LOCAL_AUTH_MIN_PASSWORD_LENGTH
					})}
				</span>
			</label>

			<label class="flex items-center gap-2">
				<input
					class="checkbox checkbox-sm"
					type="checkbox"
					disabled={saving}
					bind:checked={requirePasswordChange}
				/>
				{m.identity_access_auth_providers_local_auth_require_change()}
			</label>

			<div class="mt-2 flex flex-col justify-end gap-2 md:flex-row">
				<button
					class="btn btn-secondary"
					type="button"
					disabled={saving}
					onclick={() => dialog?.close()}
				>
					{m.common_cancel()}
				</button>
				<button class="btn btn-primary" type="submit" disabled={saving}>
					{#if saving}
						<Loading class="size-4" />
					{:else}
						{m.core_save()}
					{/if}
				</button>
			</div>
		</form>
	{/if}
</ResponsiveDialog>
