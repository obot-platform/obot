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
	let email = $state('');
	let password = $state('');
	let requirePasswordChange = $state(true);
	let error = $state<string>();
	let saving = $state(false);

	function resetForm() {
		email = '';
		password = '';
		requirePasswordChange = true;
		error = undefined;
	}

	export function open() {
		resetForm();
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
		const trimmed = email.trim();
		if (!trimmed || !password) {
			error = m.identity_access_auth_providers_local_auth_fill_email_password();
			return;
		}
		if (password.length < LOCAL_AUTH_MIN_PASSWORD_LENGTH) {
			error = m.identity_access_auth_providers_local_auth_password_min({
				min: LOCAL_AUTH_MIN_PASSWORD_LENGTH
			});
			return;
		}

		saving = true;
		error = undefined;
		try {
			await AdminService.createLocalAuthUser(trimmed, password, requirePasswordChange);
		} catch (err) {
			error = errorMessage(err);
			return;
		} finally {
			saving = false;
		}

		dialog?.close();
		resetForm();
		await onSaved();
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	class="w-full overflow-visible p-4 md:max-w-xl"
	title={m.identity_access_auth_providers_local_auth_create_new_user()}
	onClose={() => {
		error = undefined;
	}}
>
	<form class="m-4 flex flex-col gap-4 text-sm font-light md:m-0" onsubmit={save}>
		{#if error}
			<p class="text-error text-xs font-light" role="alert">{error}</p>
		{/if}

		<label class="flex flex-col gap-1" for="users-local-auth-email">
			{m.common_email()}
			<input
				id="users-local-auth-email"
				class="text-input-filled"
				type="email"
				bind:value={email}
				autocomplete="off"
				required
				disabled={saving}
				oninput={() => (error = undefined)}
			/>
		</label>

		<label class="flex flex-col gap-1" for="users-local-auth-password">
			{m.common_password()}
			<SensitiveInput
				name="users-local-auth-password"
				bind:value={password}
				autocomplete="new-password"
				minlength={LOCAL_AUTH_MIN_PASSWORD_LENGTH}
				oninput={() => (error = undefined)}
				required
				disabled={saving}
				error={!!error}
				data1pIgnore={false}
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
</ResponsiveDialog>
