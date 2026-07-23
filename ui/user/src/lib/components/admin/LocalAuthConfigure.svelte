<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
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
		TriangleAlert,
		UserPlus
	} from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		provider?: AuthProvider;
		values?: Record<string, string>;
		readonly?: boolean;
		// Save the email-domains config. Returns an error message, or undefined on success.
		onConfigure: (form: Record<string, string>) => Promise<string | undefined>;
		// Called after the modal closes, with the number of local users that currently exist.
		onClose?: (userCount: number) => void;
	}

	const { provider, values, readonly, onConfigure, onClose }: Props = $props();

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
	let savingUser = $state(false);
	let userError = $state<string>();
	let email = $state('');
	let password = $state('');
	let resettingUser = $state<LocalAuthUser>();

	export function open() {
		domains = values?.[DOMAINS_KEY] ?? '*';
		configError = undefined;
		userError = undefined;
		email = '';
		password = '';
		resettingUser = undefined;
		users = [];
		step = provider?.configured ? 'users' : 'config';
		dialog?.open();
		if (step === 'users') refreshUsers();
	}

	export function close() {
		dialog?.close();
	}

	async function handleContinue(e?: SubmitEvent) {
		e?.preventDefault();

		// A readonly admin can look but not save; just move on to view the users.
		if (readonly) {
			step = 'users';
			await refreshUsers();
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
			await refreshUsers();
		} finally {
			configuring = false;
		}
	}

	async function refreshUsers() {
		loadingUsers = true;
		userError = undefined;
		try {
			users = await AdminService.listLocalAuthUsers();
		} catch (err) {
			userError = errorMessage(err, 'Failed to load local users.');
		} finally {
			loadingUsers = false;
		}
	}

	async function handleUserSubmit(e: SubmitEvent) {
		e.preventDefault();
		savingUser = true;
		userError = undefined;
		try {
			if (resettingUser) {
				await AdminService.setLocalAuthUserPassword(resettingUser.id, password);
				resettingUser = undefined;
			} else {
				await AdminService.createLocalAuthUser(email, password);
			}
			email = '';
			password = '';
			await refreshUsers();
		} catch (err) {
			userError = errorMessage(err, 'Failed to save the user.');
		} finally {
			savingUser = false;
		}
	}

	async function handleDelete(user: LocalAuthUser) {
		savingUser = true;
		userError = undefined;
		try {
			await AdminService.deleteLocalAuthUser(user.id);
			await refreshUsers();
		} catch (err) {
			userError = errorMessage(err, 'Failed to delete the user.');
		} finally {
			savingUser = false;
		}
	}

	// The API returns errors as {"error": "..."}; surface the message rather than the raw body.
	function errorMessage(err: unknown, fallback: string) {
		if (!(err instanceof Error)) return fallback;
		return parseErrorContent(err).message || fallback;
	}
</script>

<ResponsiveDialog bind:this={dialog} class="w-xl" onClose={() => onClose?.(users.length)}>
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

	<div class="flex flex-col gap-4">
		<div class="notification-alert flex items-start gap-2">
			<TriangleAlert class="text-warning mt-0.5 size-5 shrink-0" />
			<p class="text-sm font-light">
				Local authentication stores passwords in Obot and is intended for development or testing.
				For production, use an SSO provider such as Google, GitHub, or Okta.
			</p>
		</div>

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
						Local users must have an email address in one of these domains. Use * to allow any
						domain.
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

				<div class="flex justify-end">
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
			<div class="flex flex-col gap-4">
				<div class="flex items-center justify-between gap-2">
					<h4 class="text-sm font-semibold">Users</h4>
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
				</div>

				<p class="text-muted-content text-sm font-light">
					These users sign in with an email address and password. Grant them roles from the Users
					page after their first sign-in.
				</p>

				{#if userError}
					<div class="notification-error flex items-center gap-2">
						<CircleAlert class="text-error size-5 shrink-0" />
						<p class="text-sm font-light">{userError}</p>
					</div>
				{/if}

				{#if loadingUsers}
					<div class="flex justify-center py-4"><Loading class="size-5" /></div>
				{:else if users.length === 0}
					<p class="text-muted-content py-2 text-sm font-light">
						No local users yet. Create one below so that someone can sign in.
					</p>
				{:else}
					<ul class="divide-base-300 dark:divide-base-400 divide-y">
						{#each users as user (user.id)}
							<li class="flex items-center justify-between gap-2 py-2">
								<span class="truncate text-sm">{user.email}</span>
								{#if !readonly}
									<div class="flex shrink-0 items-center gap-1">
										<IconButton
											tooltip={{ text: 'Reset password' }}
											disabled={savingUser}
											onclick={() => {
												resettingUser = user;
												password = '';
												userError = undefined;
											}}
										>
											<KeyRound class="size-4" />
										</IconButton>
										<IconButton
											variant="danger"
											tooltip={{ text: 'Delete user' }}
											disabled={savingUser}
											onclick={() => handleDelete(user)}
										>
											<Trash2 class="size-4" />
										</IconButton>
									</div>
								{/if}
							</li>
						{/each}
					</ul>
				{/if}

				{#if !readonly}
					<form
						class="border-base-300 dark:border-base-400 flex flex-col gap-3 border-t pt-4"
						onsubmit={handleUserSubmit}
					>
						<h4 class="text-sm font-semibold">
							{resettingUser ? `Reset password for ${resettingUser.email}` : 'Add a user'}
						</h4>

						{#if !resettingUser}
							<label class="flex flex-col gap-1 text-sm font-light" for="local-user-email">
								Email
								<input
									id="local-user-email"
									class="text-input-filled"
									type="email"
									bind:value={email}
									autocomplete="off"
									required
								/>
							</label>
						{/if}

						<label class="flex flex-col gap-1 text-sm font-light" for="local-user-password">
							Password
							<SensitiveInput
								name="local-user-password"
								bind:value={password}
								autocomplete="new-password"
								minlength={LOCAL_AUTH_MIN_PASSWORD_LENGTH}
								required
							/>
							<span class="text-muted-content text-xs pt-0.5">
								At least {LOCAL_AUTH_MIN_PASSWORD_LENGTH} characters. Share it with the user over a secure
								channel; they can't change it themselves yet.
							</span>
						</label>

						<div class="flex justify-end gap-2">
							{#if resettingUser}
								<button
									class="btn btn-secondary"
									type="button"
									onclick={() => {
										resettingUser = undefined;
										password = '';
									}}
								>
									Cancel
								</button>
							{/if}
							<button class="btn btn-primary" type="submit" disabled={savingUser}>
								{#if savingUser}
									<Loading class="size-4" />
								{:else if resettingUser}
									<KeyRound class="size-4" /> Reset Password
								{:else}
									<UserPlus class="size-4" /> Add User
								{/if}
							</button>
						</div>
					</form>
				{/if}

				<div class="border-base-300 dark:border-base-400 flex justify-end border-t pt-4">
					<button class="btn btn-primary" type="button" onclick={close}>Done</button>
				</div>
			</div>
		{/if}
	</div>
</ResponsiveDialog>
