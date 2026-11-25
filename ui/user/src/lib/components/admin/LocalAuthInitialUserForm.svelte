<script lang="ts">
	import FieldError from '$lib/components/FieldError.svelte';
	import RequiredMark from '$lib/components/RequiredMark.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import { LOCAL_AUTH_MIN_PASSWORD_LENGTH } from '$lib/constants';
	import { parseErrorContent } from '$lib/errors';
	import { FormValidation } from '$lib/form.svelte';
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
	const INITIAL_PASSWORD_HINT_ID = 'initial-user-password-hint';
	const INITIAL_PASSWORD_CONFIRM_ID = 'initial-user-password-confirm';
	const INITIAL_USER_ERROR_ID = 'local-auth-initial-user-error';
	const EMAIL_PATTERN = /^[^\s@]+@[^\s@]+$/;

	interface Props {
		provider?: AuthProvider;
		readonly?: boolean;
		onConfigure: (form: Record<string, string>) => Promise<string | undefined>;
		onCreated?: (userCount: number, email?: string) => void;
		onSubmitOwner?: (email: string, password: string) => Promise<string | undefined>;
		additionalActions?: Snippet;
	}

	const { provider, readonly, onConfigure, onCreated, onSubmitOwner, additionalActions }: Props =
		$props();

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

	const validation = new FormValidation<'email' | 'password' | 'confirm'>(() => {
		const email = initialEmail.trim();
		return {
			...(!email
				? { email: m.core_field_required() }
				: !EMAIL_PATTERN.test(email) && {
						email: m.identity_access_auth_providers_local_auth_email_invalid()
					}),
			...(!initialPassword
				? { password: m.core_field_required() }
				: initialPassword.length < LOCAL_AUTH_MIN_PASSWORD_LENGTH && {
						password: m.identity_access_auth_providers_local_auth_password_min({
							min: LOCAL_AUTH_MIN_PASSWORD_LENGTH
						})
					}),
			...(!initialPasswordConfirm
				? { confirm: m.core_field_required() }
				: initialPassword !== initialPasswordConfirm && {
						confirm: m.identity_access_auth_providers_local_auth_initial_user_passwords_mismatch()
					})
		};
	});

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
		initialUserError = undefined;
		if (!(await validation.check())) return;

		saving = true;
		initialUserError = undefined;
		try {
			if (!(await ensureConfigured())) {
				saving = false;
				return;
			}

			initialUserError = await (onSubmitOwner ?? createUser)(email, initialPassword);
		} catch (err) {
			initialUserError = errorMessage(
				err,
				m.identity_access_auth_providers_local_auth_initial_user_create_failed()
			);
		}

		if (initialUserError || !onSubmitOwner) saving = false;
	}

	async function createUser(email: string, password: string): Promise<string | undefined> {
		await AdminService.createLocalAuthUser(email, password, false);

		let error: string | undefined;
		try {
			userCount = (await AdminService.listLocalAuthUsers()).length;
		} catch (err) {
			error = errorMessage(
				err,
				m.identity_access_auth_providers_local_auth_initial_user_list_failed()
			);
			userCount = 1;
		}
		onCreated?.(userCount, email);
		return error;
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

<form
	{@attach validation.attach}
	class="flex flex-col gap-4"
	novalidate
	onsubmit={handleCreateInitialUser}
>
	{#if configError}
		<div class="notification-error flex items-center gap-2" role="alert">
			<CircleAlert class="text-error size-5 shrink-0" />
			<p class="text-sm font-light">{configError}</p>
		</div>
	{/if}

	<div class="flex flex-col gap-1 text-sm font-light">
		<label class:error={validation.has('email')} for={INITIAL_EMAIL_ID}>
			{m.common_email()}
			<RequiredMark invalid={validation.has('email')} />
		</label>
		<input
			{...validation.attrs('email', INITIAL_EMAIL_ID)}
			name="email"
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
			autocapitalize="none"
			autocorrect="off"
			spellcheck="false"
			required
			disabled={saving}
			class:error={validation.has('email')}
		/>
		<FieldError id={INITIAL_EMAIL_ID} message={validation.message('email')} />
	</div>

	<div class="flex flex-col gap-1 text-sm font-light">
		<label class:error={validation.has('password')} for={INITIAL_PASSWORD_ID}>
			{m.common_password()}
			<RequiredMark invalid={validation.has('password')} />
		</label>
		<SensitiveInput
			describedby={[
				INITIAL_PASSWORD_HINT_ID,
				validation.has('password') && `${INITIAL_PASSWORD_ID}-error`
			]
				.filter(Boolean)
				.join(' ')}
			name={INITIAL_PASSWORD_ID}
			bind:value={initialPassword}
			autocomplete="new-password"
			minlength={LOCAL_AUTH_MIN_PASSWORD_LENGTH}
			oninput={() => (initialUserError = undefined)}
			required
			disabled={saving}
			error={validation.has('password')}
			data1pIgnore={false}
		/>
		<span id={INITIAL_PASSWORD_HINT_ID} class="text-muted-content min-h-4 pt-0.5 text-xs">
			{m.identity_access_auth_providers_local_auth_initial_user_password_min_required({
				min: LOCAL_AUTH_MIN_PASSWORD_LENGTH
			})}
		</span>
		<FieldError id={INITIAL_PASSWORD_ID} class="pt-0.5" message={validation.message('password')} />
	</div>

	<div class="flex flex-col gap-1 text-sm font-light">
		<label class:error={validation.has('confirm')} for={INITIAL_PASSWORD_CONFIRM_ID}>
			{m.identity_access_auth_providers_local_auth_initial_user_confirm_password()}
			<RequiredMark invalid={validation.has('confirm')} />
		</label>
		<SensitiveInput
			describedby={validation.message('confirm')
				? `${INITIAL_PASSWORD_CONFIRM_ID}-error`
				: undefined}
			name={INITIAL_PASSWORD_CONFIRM_ID}
			bind:value={initialPasswordConfirm}
			autocomplete="new-password"
			minlength={LOCAL_AUTH_MIN_PASSWORD_LENGTH}
			oninput={() => (initialUserError = undefined)}
			required
			disabled={saving}
			error={validation.has('confirm')}
			data1pIgnore={false}
		/>
		<FieldError id={INITIAL_PASSWORD_CONFIRM_ID} message={validation.message('confirm')} />
	</div>

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
