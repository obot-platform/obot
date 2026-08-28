<script lang="ts">
	import ResponsiveDialog, {
		type ResponsiveDialogAnimate
	} from '$lib/components/ResponsiveDialog.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { MultiValueInput } from '$lib/components/ui/multi-value-input';
	import { LOCAL_AUTH_MIN_PASSWORD_LENGTH } from '$lib/constants';
	import { parseErrorContent } from '$lib/errors';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService, type AuthProvider, type LocalAuthUser } from '$lib/services';
	import { darkMode } from '$lib/stores';
	import {
		ArrowLeft,
		CircleAlert,
		KeyRound,
		Trash2,
		Undo2,
		UserPlus,
		Check,
		X
	} from '@lucide/svelte';
	import type { Snippet } from 'svelte';
	import { SvelteMap, SvelteSet } from 'svelte/reactivity';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		provider?: AuthProvider;
		values?: Record<string, string>;
		readonly?: boolean;
		// Save the email-domains config. Returns an error message, or undefined on success.
		onConfigure: (form: Record<string, string>) => Promise<string | undefined>;
		// Called after the modal closes, with the number of local users that currently exist.
		onClose?: (userCount: number) => void;
		animate?: ResponsiveDialogAnimate;
		required?: boolean;
		additionalActions?: Snippet;
	}

	const {
		provider,
		values,
		readonly,
		onConfigure,
		onClose,
		animate,
		required,
		additionalActions
	}: Props = $props();

	const DOMAINS_KEY = 'OBOT_AUTH_PROVIDER_EMAIL_DOMAINS';

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	// The modal is a two-step flow: configure the provider, then manage its users. Users can only
	// be created once the provider is configured (the server needs the allowed-domains credential),
	// so an unconfigured provider opens on 'config' and a configured one opens straight on 'users'.
	let step = $state<'config' | 'users'>('config');
	let domains = $state('');
	let configuring = $state(false);
	let configError = $state<string>();

	let users = $state<LocalAuthUser[]>([]);
	let loadingUsers = $state(false);
	let saving = $state(false);

	let userError = $state<string[]>();
	let newUserError = $state<string>();
	let draftError = $state<string>();

	let newUsers = $state<{ email: string; password: string }[]>([]);
	let draftingNewUser = $state(false);
	let draftEmail = $state('');
	let draftPassword = $state('');
	let shakingDraft = $state(false);
	let draftReset = $state<{ id: LocalAuthUser['id']; password: string }>();
	let shakingReset = $state(false);
	let resetPassword = new SvelteMap<LocalAuthUser['id'], string>();
	let deleteUsers = new SvelteSet<LocalAuthUser['id']>();

	let shakeTimeout: ReturnType<typeof setTimeout> | undefined;

	const DRAFT_EMAIL_ID = 'local-user-email-draft';
	const DRAFT_PASSWORD_ID = 'local-user-password-draft';
	const DRAFT_CONFIRM_ID = 'local-user-confirm-draft';
	const NEW_USER_ERROR_ID = 'local-auth-new-user-error';
	const DRAFT_RESET_ERROR_ID = 'local-auth-draft-reset-error';

	const hasPendingChanges = $derived(
		newUsers.length > 0 || resetPassword.size > 0 || deleteUsers.size > 0
	);

	export function open() {
		domains = values?.[DOMAINS_KEY] ?? '*';
		configError = undefined;
		userError = undefined;
		newUserError = undefined;
		draftError = undefined;
		newUsers = [];
		draftingNewUser = false;
		draftEmail = '';
		draftPassword = '';
		draftReset = undefined;
		shakingDraft = false;
		shakingReset = false;
		clearTimeout(shakeTimeout);
		resetPassword.clear();
		deleteUsers.clear();
		users = [];
		step = provider?.configured ? 'users' : 'config';
		dialog?.open();
		if (step === 'users') showUsers();
	}

	export function close() {
		dialog?.close();
	}

	async function handleContinue(e?: SubmitEvent) {
		e?.preventDefault();

		// A readonly admin can look but not save; just move on to view the users.
		if (readonly) {
			step = 'users';
			await showUsers();
			return;
		}

		if (!domains.trim()) {
			configError = 'Enter at least one allowed email domain (use * to allow any).';
			return;
		}

		configuring = true;
		configError = undefined;
		try {
			const err = await onConfigure({ [DOMAINS_KEY]: domains });
			if (err) {
				configError = err;
				return;
			}
			step = 'users';
			await showUsers();
		} finally {
			configuring = false;
		}
	}

	// Entering the users step with nothing to manage: skip the empty state and open the draft form,
	// since creating a user is the only thing left to do here.
	async function showUsers() {
		await refreshUsers();
		if (readonly || userError || users.length > 0) return;
		addNewUser();
	}

	async function refreshUsers(forwardError?: string) {
		loadingUsers = true;
		userError = forwardError ? [forwardError] : undefined;
		try {
			users = await AdminService.listLocalAuthUsers();
		} catch (err) {
			const loadError = errorMessage(err, 'Failed to load local users.');
			userError = [...(forwardError ? [forwardError] : []), loadError];
		} finally {
			loadingUsers = false;
		}
	}

	function prefersReducedMotion() {
		return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
	}

	function announceNewUserError(message: string) {
		newUserError = undefined;
		queueMicrotask(() => {
			newUserError = message;
		});
	}

	function announceDraftError(message: string) {
		draftError = undefined;
		queueMicrotask(() => {
			draftError = message;
		});
	}

	function shake(kind: 'new-user' | 'reset') {
		if (prefersReducedMotion()) return;

		if (kind === 'new-user') {
			shakingDraft = false;
			requestAnimationFrame(() => {
				shakingDraft = true;
				clearTimeout(shakeTimeout);
				shakeTimeout = setTimeout(() => {
					shakingDraft = false;
				}, 400);
			});
			return;
		}

		shakingReset = false;
		requestAnimationFrame(() => {
			shakingReset = true;
			clearTimeout(shakeTimeout);
			shakeTimeout = setTimeout(() => {
				shakingReset = false;
			}, 400);
		});
	}

	function focusDraftField() {
		if (!draftingNewUser) return;

		const emailEl = document.getElementById(DRAFT_EMAIL_ID) as HTMLInputElement | null;
		const passwordEl = document.getElementById(DRAFT_PASSWORD_ID) as HTMLInputElement | null;
		const confirmEl = document.getElementById(DRAFT_CONFIRM_ID);

		const emailReady = Boolean(draftEmail.trim() && emailEl?.checkValidity());
		const passwordReady = draftPassword.length >= LOCAL_AUTH_MIN_PASSWORD_LENGTH;
		const target = !emailReady ? emailEl : !passwordReady ? passwordEl : confirmEl;
		const scrollBehavior = prefersReducedMotion() ? 'auto' : 'smooth';

		target?.scrollIntoView({ block: 'nearest', behavior: scrollBehavior });
		target?.focus();
	}

	function focusResetField() {
		if (!draftReset) return;

		const passwordId = `reset-password-${draftReset.id}`;
		const confirmId = `reset-confirm-${draftReset.id}`;
		const passwordEl = document.getElementById(passwordId) as HTMLInputElement | null;
		const confirmEl = document.getElementById(confirmId);
		const passwordReady = draftReset.password.length >= LOCAL_AUTH_MIN_PASSWORD_LENGTH;
		const target = !passwordReady ? passwordEl : confirmEl;
		const scrollBehavior = prefersReducedMotion() ? 'auto' : 'smooth';

		target?.scrollIntoView({ block: 'nearest', behavior: scrollBehavior });
		target?.focus();
	}

	function attentionDraftNewUser(message: string) {
		announceNewUserError(message);
		shake('new-user');
		queueMicrotask(() => focusDraftField());
	}

	function attentionDraftReset(message: string) {
		announceDraftError(message);
		shake('reset');
		queueMicrotask(() => focusResetField());
	}

	function addNewUser() {
		if (draftingNewUser) {
			attentionDraftNewUser('Finish or remove the new user before adding another.');
			return;
		}
		if (draftReset) {
			attentionDraftReset('Finish or cancel the password reset before adding a new user.');
			return;
		}
		newUserError = undefined;
		draftEmail = '';
		draftPassword = '';
		draftingNewUser = true;
	}

	function isCommitKey(e: KeyboardEvent) {
		return e.key === 'Enter' && !e.isComposing && e.keyCode !== 229;
	}

	function handleDraftNewUserKeydown(e: KeyboardEvent) {
		if (!isCommitKey(e)) return;
		e.preventDefault();
		confirmNewUser();
	}

	function handleDraftResetKeydown(e: KeyboardEvent) {
		if (!isCommitKey(e)) return;
		e.preventDefault();
		confirmResetPassword();
	}

	function confirmNewUser(): boolean {
		if (!draftingNewUser) return false;

		if (!draftEmail.trim() || !draftPassword) {
			attentionDraftNewUser('Fill out the required email and password fields.');
			return false;
		}
		if (draftPassword.length < LOCAL_AUTH_MIN_PASSWORD_LENGTH) {
			attentionDraftNewUser(
				`Passwords must be at least ${LOCAL_AUTH_MIN_PASSWORD_LENGTH} characters.`
			);
			return false;
		}

		newUsers.push({
			email: draftEmail.trim(),
			password: draftPassword
		});
		draftingNewUser = false;
		draftEmail = '';
		draftPassword = '';
		newUserError = undefined;
		return true;
	}

	function cancelDraftNewUser() {
		draftingNewUser = false;
		draftEmail = '';
		draftPassword = '';
		newUserError = undefined;
	}

	function removeNewUser(index: number) {
		if (draftingNewUser) {
			attentionDraftNewUser('Apply changes or remove the new user.');
			return;
		}
		newUsers.splice(index, 1);
	}

	function startResetPassword(user: LocalAuthUser) {
		if (draftingNewUser) {
			attentionDraftNewUser('Finish or remove the new user before updating an existing user.');
			return;
		}
		if (draftReset) {
			attentionDraftReset(
				draftReset.id === user.id
					? 'Confirm or cancel this password reset first.'
					: 'Finish or cancel the password reset before resetting another user.'
			);
			return;
		}

		draftError = undefined;
		draftReset = {
			id: user.id,
			password: resetPassword.get(user.id) ?? ''
		};
	}

	function confirmResetPassword(): boolean {
		if (!draftReset) return false;

		if (!draftReset.password || draftReset.password.length < LOCAL_AUTH_MIN_PASSWORD_LENGTH) {
			attentionDraftReset(
				!draftReset.password
					? 'Fill out the required password field.'
					: `Passwords must be at least ${LOCAL_AUTH_MIN_PASSWORD_LENGTH} characters.`
			);
			return false;
		}

		resetPassword.set(draftReset.id, draftReset.password);
		draftReset = undefined;
		draftError = undefined;
		return true;
	}

	function cancelResetPassword() {
		draftReset = undefined;
		draftError = undefined;
	}

	function markDeleted(user: LocalAuthUser) {
		if (draftingNewUser) {
			attentionDraftNewUser('Finish or remove the new user before updating an existing user.');
			return;
		}
		if (draftReset && draftReset.id !== user.id) {
			attentionDraftReset('Finish or cancel the password reset before updating another user.');
			return;
		}

		if (draftReset?.id === user.id) {
			draftReset = undefined;
			draftError = undefined;
		}
		deleteUsers.add(user.id);
		resetPassword.delete(user.id);
	}

	function undoDelete(user: LocalAuthUser) {
		deleteUsers.delete(user.id);
	}

	function validatePending(): string | undefined {
		if (draftingNewUser) {
			return 'Finish or remove the new user before saving.';
		}
		if (draftReset) {
			return 'Finish or cancel the password reset before saving.';
		}
		for (const user of newUsers) {
			if (!user.email.trim()) {
				return 'Every new user needs an email address.';
			}
			if (user.password.length < LOCAL_AUTH_MIN_PASSWORD_LENGTH) {
				return `Passwords must be at least ${LOCAL_AUTH_MIN_PASSWORD_LENGTH} characters.`;
			}
		}
		for (const [id, password] of resetPassword) {
			if (deleteUsers.has(id)) continue;
			if (password.length < LOCAL_AUTH_MIN_PASSWORD_LENGTH) {
				return `Passwords must be at least ${LOCAL_AUTH_MIN_PASSWORD_LENGTH} characters.`;
			}
		}
		return undefined;
	}

	async function handleSave(e: SubmitEvent) {
		e.preventDefault();

		if (readonly || (!hasPendingChanges && !draftingNewUser && !draftReset)) {
			close();
			return;
		}

		if (draftingNewUser && !confirmNewUser()) {
			return;
		}

		if (draftReset && !confirmResetPassword()) {
			return;
		}

		const validationError = validatePending();
		if (validationError) {
			userError = [validationError];
			return;
		}

		saving = true;
		userError = undefined;
		try {
			for (const id of [...deleteUsers]) {
				await AdminService.deleteLocalAuthUser(id);
				deleteUsers.delete(id);
			}

			for (const [id, password] of [...resetPassword]) {
				await AdminService.setLocalAuthUserPassword(id, password);
				resetPassword.delete(id);
			}

			while (newUsers.length > 0) {
				const user = newUsers[0];
				await AdminService.createLocalAuthUser(user.email.trim(), user.password);
				newUsers.shift();
			}
			await refreshUsers();
			close();
		} catch (err) {
			await refreshUsers(errorMessage(err, 'Failed to save all changes.'));
		} finally {
			saving = false;
		}
	}

	// The API returns errors as {"error": "..."}; surface the message rather than the raw body.
	function errorMessage(err: unknown, fallback: string) {
		if (!(err instanceof Error)) return fallback;
		return parseErrorContent(err).message || fallback;
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	class={twMerge('w-xl', step === 'users' && 'max-h-[calc(100dvh-1rem)]')}
	onClose={() => onClose?.(users.length)}
	{animate}
	disableClickOutside={required}
	hideClose={required}
>
	{#snippet titleContent()}
		<div class="flex items-center gap-2">
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
			Set Up {provider?.name}
		</div>
	{/snippet}

	{@render infoContent()}

	{#if step === 'config'}
		<form class="flex flex-col gap-4" onsubmit={handleContinue}>
			{#if configError}
				<div class="notification-error flex items-center gap-2">
					<CircleAlert class="text-error size-5 shrink-0" />
					<p class="text-sm font-light">{configError}</p>
				</div>
			{/if}

			<div class="flex flex-col gap-1">
				<label for="local-auth-domains">Allowed Email Domains</label>
				<span class="text-gray text-xs">
					Local users must have an email address in one of these domains. Use * to allow any domain.
				</span>
				<MultiValueInput
					bind:value={domains}
					id="local-auth-domains"
					labels={{ '*': 'All domains' }}
					class="text-input-filled"
					placeholder={`Hit "Enter" to insert`.toString()}
					disabled={readonly}
				/>
			</div>

			<div class="flex justify-between">
				<div>
					{#if additionalActions}
						{@render additionalActions?.()}
					{/if}
				</div>
				<button class="btn btn-primary" type="submit" disabled={configuring}>
					{#if configuring}
						<Loading class="size-4" />
					{:else}
						Continue
					{/if}
				</button>
			</div>
		</form>
	{:else}
		<div class="flex flex-col gap-2 grow">
			{#if !readonly}
				<button
					class="text-link flex items-center gap-1 text-xs font-light"
					type="button"
					onclick={() => {
						configError = undefined;
						step = 'config';
					}}
				>
					<ArrowLeft class="size-3.5" /> Configuration
				</button>
			{/if}

			<p class="text-muted-content text-sm font-light">
				These users sign in with an email address and password. Grant them roles from the Users page
				after their first sign-in.
			</p>

			<form class="flex flex-col gap-4 grow max-w-full overflow-hidden" onsubmit={handleSave}>
				<div class="flex items-center justify-between gap-2">
					<h4 class="text-sm font-semibold">Users</h4>
					{#if !readonly}
						{@render addNewUserButton()}
					{/if}
				</div>

				{#if userError}
					<div class="notification-error flex items-center gap-2" role="alert">
						<CircleAlert class="text-error size-5 shrink-0" />
						<p class="text-sm font-light">{userError.join('\n')}</p>
					</div>
				{/if}

				{#if loadingUsers}
					<div class="flex justify-center py-4"><Loading class="size-5" /></div>
				{:else if users.length === 0 && newUsers.length === 0 && !draftingNewUser}
					<p class="text-muted-content py-2 text-center text-xs font-light">
						No local users yet. Click {@render addNewUserButton(
							'inline-block justify-items-center mx-1',
							'Create New User'
						)} to create a user.
					</p>
				{:else}
					<ul class="flex flex-col gap-1">
						{#each users as user (user.id)}
							{@const isDeleted = deleteUsers.has(user.id)}
							{@const isDraftResetting = draftReset?.id === user.id}
							{@const hasPendingReset = resetPassword.has(user.id)}
							<li
								class="flex flex-col gap-2 py-2 border-base-300 dark:border-base-400 rounded-md border p-3"
								class:draft-shake={shakingReset && isDraftResetting}
							>
								<div class="flex items-center justify-between gap-2">
									<span
										class={twMerge(
											'truncate text-sm',
											isDeleted && 'text-muted-content line-through'
										)}
									>
										{user.email}
									</span>
									{#if !readonly}
										<div class="flex shrink-0 items-center gap-1">
											{#if isDeleted}
												<IconButton
													tooltip={{ text: 'Undo delete', disablePortal: true }}
													disabled={saving}
													onclick={() => undoDelete(user)}
												>
													<Undo2 class="size-4" />
												</IconButton>
											{:else if isDraftResetting}
												<IconButton
													id={'reset-confirm-' + user.id}
													variant="primary"
													tooltip={{ text: 'Confirm password reset', disablePortal: true }}
													disabled={saving}
													onclick={confirmResetPassword}
												>
													<Check class="size-4" />
												</IconButton>
												<IconButton
													variant="danger"
													tooltip={{ text: 'Cancel password reset', disablePortal: true }}
													disabled={saving}
													onclick={cancelResetPassword}
												>
													<X class="size-4" />
												</IconButton>
											{:else}
												<IconButton
													tooltip={{
														text: hasPendingReset ? 'Edit password reset' : 'Reset password',
														disablePortal: true
													}}
													disabled={saving}
													onclick={() => startResetPassword(user)}
												>
													<KeyRound class="size-4" />
												</IconButton>
												<IconButton
													variant="danger"
													tooltip={{ text: 'Delete user', disablePortal: true }}
													disabled={saving}
													onclick={() => markDeleted(user)}
												>
													<Trash2 class="size-4" />
												</IconButton>
											{/if}
										</div>
									{/if}
								</div>

								{#if isDeleted}
									<p class="text-muted-content text-xs">
										Marked for deletion. Click undo to keep this user, or Save to confirm.
									</p>
								{:else if isDraftResetting && draftReset}
									<div class="flex flex-col gap-3">
										<label
											class="flex flex-col gap-1 text-sm font-light"
											for="reset-password-{user.id}"
										>
											New password
											<SensitiveInput
												name="reset-password-{user.id}"
												bind:value={draftReset.password}
												autocomplete="new-password"
												minlength={LOCAL_AUTH_MIN_PASSWORD_LENGTH}
												required
												disabled={saving}
												error={!!draftError}
												data1pIgnore={false}
												oninput={() => (draftError = undefined)}
												onkeydown={handleDraftResetKeydown}
											/>
											<span class="text-muted-content pt-0.5 text-xs">
												At least {LOCAL_AUTH_MIN_PASSWORD_LENGTH} characters. Share it with the user over
												a secure channel; they can't change it themselves yet.
											</span>
										</label>

										{#if draftError}
											<p
												id={DRAFT_RESET_ERROR_ID}
												class="text-error text-xs font-light"
												role="alert"
											>
												{draftError}
											</p>
										{/if}
									</div>
								{:else if hasPendingReset}
									<p class="text-muted-content text-xs">
										Password reset pending. Save to apply, or edit to change it.
									</p>
								{/if}
							</li>
						{/each}

						{#each newUsers as user, i (user.email + String(i))}
							<li
								class="flex items-center justify-between gap-2 py-2 border-base-300 dark:border-base-400 rounded-md border p-3"
							>
								<span class="truncate text-sm">{user.email}</span>
								<IconButton
									variant="danger"
									tooltip={{ text: 'Remove', disablePortal: true }}
									disabled={saving}
									onclick={() => removeNewUser(i)}
								>
									<X class="size-4" />
								</IconButton>
							</li>
						{/each}
					</ul>

					{#if draftingNewUser}
						<div
							class="border-base-300 dark:border-base-400 mt-1 flex flex-col gap-3 rounded-md border p-3"
							class:draft-shake={shakingDraft}
						>
							<div class="flex items-center justify-between gap-2">
								<span class="text-sm font-medium">New user</span>
								<div class="flex shrink-0 items-center gap-1">
									<IconButton
										id={DRAFT_CONFIRM_ID}
										variant="primary"
										tooltip={{ text: 'Confirm', disablePortal: true }}
										disabled={saving}
										onclick={confirmNewUser}
									>
										<Check class="size-4" />
									</IconButton>
									<IconButton
										variant="danger"
										tooltip={{ text: 'Remove', disablePortal: true }}
										disabled={saving}
										onclick={cancelDraftNewUser}
									>
										<X class="size-4" />
									</IconButton>
								</div>
							</div>

							<label class="flex flex-col gap-1 text-sm font-light" for={DRAFT_EMAIL_ID}>
								Email
								<input
									id={DRAFT_EMAIL_ID}
									class="text-input-filled"
									type="email"
									bind:value={draftEmail}
									autocomplete="off"
									required
									disabled={saving}
									oninput={() => (newUserError = undefined)}
									onkeydown={handleDraftNewUserKeydown}
									aria-invalid={newUserError ? 'true' : undefined}
									aria-describedby={newUserError ? NEW_USER_ERROR_ID : undefined}
									class:error={!!newUserError}
								/>
							</label>

							<label class="flex flex-col gap-1 text-sm font-light" for={DRAFT_PASSWORD_ID}>
								Password
								<SensitiveInput
									name={DRAFT_PASSWORD_ID}
									bind:value={draftPassword}
									autocomplete="new-password"
									minlength={LOCAL_AUTH_MIN_PASSWORD_LENGTH}
									oninput={() => (newUserError = undefined)}
									onkeydown={handleDraftNewUserKeydown}
									required
									disabled={saving}
									error={!!newUserError}
									data1pIgnore={false}
								/>
								<span class="text-muted-content pt-0.5 text-xs">
									At least {LOCAL_AUTH_MIN_PASSWORD_LENGTH} characters. Share it with the user over a
									secure channel; they can't change it themselves yet.
								</span>
							</label>

							{#if newUserError}
								<p id={NEW_USER_ERROR_ID} class="text-error text-xs font-light" role="alert">
									{newUserError}
								</p>
							{/if}
						</div>
					{/if}
				{/if}

				<div class="flex grow"></div>

				<div
					class="sticky bottom-0 left-0 w-full border-base-300 dark:border-base-400 flex justify-end border-t pt-4"
				>
					{#if readonly}
						<button class="btn btn-primary" type="button" onclick={close}>Close</button>
					{:else}
						<button class="btn btn-primary" type="submit" disabled={saving || loadingUsers}>
							{#if saving}
								<Loading class="size-4" />
							{:else}
								Save
							{/if}
						</button>
					{/if}
				</div>
			</form>
		</div>
	{/if}
</ResponsiveDialog>

{#snippet infoContent()}
	<div class="notification-info flex flex-col items-start gap-1 mb-4">
		<div class="flex items-center gap-1">
			<p class="text-sm font-semibold">Set up initially with local authentication!</p>
		</div>
		<div>
			<p class="text-xs font-light">
				Once you're ready for production, we support other authentication providers such as Google,
				GitHub, and Okta, or get access to additional authentication providers such as Entra, Okta,
				JumpCloud, and Auth0, with a one-time registration.
			</p>
		</div>
	</div>
{/snippet}

{#snippet addNewUserButton(klass?: string, label = 'Add New User')}
	<IconButton
		tooltip={{ text: label, disablePortal: true }}
		disabled={saving}
		onclick={addNewUser}
		variant="primary"
		class={klass}
	>
		<UserPlus class="size-4" />
	</IconButton>
{/snippet}

<style>
	@keyframes draft-shake {
		0%,
		100% {
			transform: translateX(0);
		}
		20% {
			transform: translateX(-4px);
		}
		40% {
			transform: translateX(4px);
		}
		60% {
			transform: translateX(-3px);
		}
		80% {
			transform: translateX(3px);
		}
	}

	.draft-shake {
		animation: draft-shake 0.35s ease-in-out;
	}

	@media (prefers-reduced-motion: reduce) {
		.draft-shake {
			animation: none;
		}
	}
</style>
