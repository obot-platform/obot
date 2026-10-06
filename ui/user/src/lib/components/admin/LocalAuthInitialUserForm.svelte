<script lang="ts">
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import { LOCAL_AUTH_MIN_PASSWORD_LENGTH } from '$lib/constants';
	import { parseErrorContent } from '$lib/errors';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService, type AuthProvider } from '$lib/services';
	import { darkMode } from '$lib/stores';
	import { CircleAlert } from '@lucide/svelte';
	import type { Snippet } from 'svelte';
	import { onMount } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	const DOMAINS_KEY = 'OBOT_AUTH_PROVIDER_EMAIL_DOMAINS';
	const INITIAL_EMAIL_ID = 'initial-user-email';
	const INITIAL_PASSWORD_ID = 'initial-user-password';
	const INITIAL_PASSWORD_CONFIRM_ID = 'initial-user-password-confirm';
	const INITIAL_USER_ERROR_ID = 'local-auth-initial-user-error';

	interface Props {
		provider?: AuthProvider;
		readonly?: boolean;
		onConfigure: (form: Record<string, string>) => Promise<string | undefined>;
		onCreated?: (userCount: number, email?: string) => void;
		additionalActions?: Snippet;
	}

	const { provider, readonly, onConfigure, onCreated, additionalActions }: Props = $props();

	let configuring = $state(false);
	let configError = $state<string>();
	let saving = $state(false);
	let initialEmail = $state('');
	let initialPassword = $state('');
	let initialPasswordConfirm = $state('');
	let initialUserError = $state<string>();
	let userCount = $state(0);
	let configurePromise: Promise<boolean> | undefined;

	onMount(() => {
		configurePromise = provider?.configured ? Promise.resolve(true) : autoConfigure();
	});

	async function autoConfigure(): Promise<boolean> {
		configuring = true;
		configError = undefined;
		try {
			const err = await onConfigure({ [DOMAINS_KEY]: '*' });
			if (err) {
				configError = err;
				return false;
			}
			return true;
		} finally {
			configuring = false;
		}
	}

	async function ensureConfigured(): Promise<boolean> {
		if (configurePromise) {
			const ok = await configurePromise;
			if (ok) return true;
		}
		configurePromise = autoConfigure();
		return configurePromise;
	}

	function errorMessage(err: unknown, fallback: string) {
		if (!(err instanceof Error)) return fallback;
		return parseErrorContent(err).message || fallback;
	}

	async function handleCreateInitialUser(e: SubmitEvent) {
		e.preventDefault();
		if (readonly || saving) {
			if (readonly) onCreated?.(userCount);
			return;
		}

		const email = initialEmail.trim();
		if (!email || !initialPassword || !initialPasswordConfirm) {
			initialUserError = m.identity_access_auth_providers_local_auth_fill_email_password();
			return;
		}
		if (initialPassword.length < LOCAL_AUTH_MIN_PASSWORD_LENGTH) {
			initialUserError = m.identity_access_auth_providers_local_auth_password_min({
				min: LOCAL_AUTH_MIN_PASSWORD_LENGTH
			});
			return;
		}
		if (initialPassword !== initialPasswordConfirm) {
			initialUserError =
				m.identity_access_auth_providers_local_auth_initial_user_passwords_mismatch();
			return;
		}

		saving = true;
		initialUserError = undefined;
		try {
			if (!(await ensureConfigured())) {
				saving = false;
				return;
			}

			await AdminService.createLocalAuthUser(email, initialPassword, false);
		} catch (err) {
			saving = false;
			initialUserError = errorMessage(
				err,
				m.identity_access_auth_providers_local_auth_initial_user_create_failed()
			);
		}

		if (!initialUserError) {
			try {
				const users = await AdminService.listLocalAuthUsers();
				userCount = users.length;
			} catch (err) {
				initialUserError = errorMessage(
					err,
					m.identity_access_auth_providers_local_auth_initial_user_list_failed()
				);
				userCount = 1;
			} finally {
				onCreated?.(userCount, email);
				saving = false;
			}
		}
	}
</script>

<div class="mb-4 flex items-center gap-2">
	{#if darkMode.isDark}
		{@const url = provider?.iconDark ?? provider?.icon}
		<img
			src={url}
			alt={provider?.name}
			class={twMerge('size-9 rounded-md p-1', !provider?.iconDark && 'bg-base-300')}
		/>
	{:else}
		<img src={provider?.icon} alt={provider?.name} class="bg-base-200 size-9 rounded-md p-1" />
	{/if}
	<h2 class="text-lg font-semibold">
		{m.identity_access_auth_providers_local_auth_initial_user_title()}
	</h2>
</div>

<div class="notification-info mb-4 flex flex-col items-start gap-1">
	<div class="flex items-center gap-1">
		<p class="text-sm font-semibold">
			{m.identity_access_auth_providers_local_auth_initial_user_intro()}
		</p>
	</div>
	<div>
		<p class="text-xs font-light">
			{m.identity_access_auth_providers_local_auth_initial_user_other_providers()}
		</p>
	</div>
</div>

<form class="flex flex-col gap-4" onsubmit={handleCreateInitialUser}>
	{#if configError}
		<div class="notification-error flex items-center gap-2" role="alert">
			<CircleAlert class="text-error size-5 shrink-0" />
			<p class="text-sm font-light">{configError}</p>
		</div>
	{/if}

	<label class="flex flex-col gap-1 text-sm font-light" for={INITIAL_EMAIL_ID}>
		{m.common_email()}
		<input
			id={INITIAL_EMAIL_ID}
			class="text-input-filled"
			type="email"
			bind:value={
				() => initialEmail,
				(v) => {
					initialEmail = v;
					initialUserError = undefined;
				}
			}
			autocomplete="email"
			required
			disabled={saving}
			aria-invalid={initialUserError ? 'true' : undefined}
			aria-describedby={initialUserError ? INITIAL_USER_ERROR_ID : undefined}
			class:error={!!initialUserError}
		/>
	</label>

	<label class="flex flex-col gap-1 text-sm font-light" for={INITIAL_PASSWORD_ID}>
		{m.common_password()}
		<SensitiveInput
			name={INITIAL_PASSWORD_ID}
			bind:value={initialPassword}
			autocomplete="new-password"
			minlength={LOCAL_AUTH_MIN_PASSWORD_LENGTH}
			oninput={() => (initialUserError = undefined)}
			required
			disabled={saving}
			error={!!initialUserError}
			data1pIgnore={false}
		/>
		<span class="text-muted-content min-h-4 pt-0.5 text-xs">
			{m.identity_access_auth_providers_local_auth_initial_user_password_min_required({
				min: LOCAL_AUTH_MIN_PASSWORD_LENGTH
			})}
		</span>
	</label>

	<label class="flex flex-col gap-1 text-sm font-light" for={INITIAL_PASSWORD_CONFIRM_ID}>
		{m.identity_access_auth_providers_local_auth_initial_user_confirm_password()}
		<SensitiveInput
			name={INITIAL_PASSWORD_CONFIRM_ID}
			bind:value={initialPasswordConfirm}
			autocomplete="new-password"
			minlength={LOCAL_AUTH_MIN_PASSWORD_LENGTH}
			oninput={() => (initialUserError = undefined)}
			required
			disabled={saving}
			error={!!initialUserError}
			data1pIgnore={false}
		/>
	</label>

	<p
		id={INITIAL_USER_ERROR_ID}
		class="text-error min-h-4 text-xs font-light"
		role={initialUserError ? 'alert' : undefined}
		aria-hidden={initialUserError ? undefined : true}
	>
		{initialUserError ?? ''}
	</p>

	<div class="flex justify-between">
		<div>
			{#if additionalActions}
				{@render additionalActions?.()}
			{/if}
		</div>
		<button class="btn btn-primary" type="submit" disabled={saving || configuring}>
			{#if saving}
				<Loading class="size-4" />
			{:else}
				{m.core_continue()}
			{/if}
		</button>
	</div>
</form>
