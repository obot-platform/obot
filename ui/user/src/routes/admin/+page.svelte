<script lang="ts">
	import { resolve } from '$app/paths';
	import Navbar from '$lib/components/Navbar.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import BetaLogo from '$lib/components/navbar/BetaLogo.svelte';
	import { SCIM_VIEW_PATH, SEEN_SPLASH_DIALOG_KEY } from '$lib/constants';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { navigateTo, reloadPage } from '$lib/navigation';
	import {
		AdminService,
		UserService,
		type AuthProvider,
		type BootstrapStatus,
		type TempUser
	} from '$lib/services';
	import { clearProductAnalyticsConsentDeferral } from '$lib/stores/productTelemetryConsent.svelte';
	import { goto } from '$lib/url';
	import { CircleAlert, Handshake, ShieldAlert } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { slide } from 'svelte/transition';

	const { data } = $props();
	let { authProviders, loggedIn, hasAccess, showSetupHandoff } = $derived(data);
	let fetchBootstrapStatus = $state<Promise<BootstrapStatus>>();
	let bootstrapToken = $state('');
	let error = $state('');
	let showBootstrapLogin = $derived(authProviders.length === 0);
	let tempDataPromises =
		$state<Promise<[TempUser, Awaited<ReturnType<typeof AdminService.listExplicitRoleEmails>>]>>();
	let loadingCancelTempUser = $state(false);
	let loadingConfirmTempUser = $state(false);
	let showSuccessOwnerConfirmation = $state(false);
	// The configured auth provider, when it provisions users and groups through SCIM. Its setup
	// continues on the SCIM sub-tab once the new Owner signs in.
	let scimProvider = $state<AuthProvider>();
	// Signing out lands on the sign-in page, which returns the new Owner to the SCIM sub-tab once they
	// sign in. It is sent there directly, because a page that needs a session keeps only its path
	// when it sends a signed-out browser to sign in.
	let afterSignOut = $derived(
		scimProvider ? `/?rd=${encodeURIComponent(SCIM_VIEW_PATH)}` : '/admin'
	);

	onMount(() => {
		fetchBootstrapStatus = UserService.getBootstrapStatus();
		if (showSetupHandoff) {
			tempDataPromises = Promise.all([
				AdminService.getTempUser(),
				AdminService.listExplicitRoleEmails()
			]);
			AdminService.listAuthProviders()
				.then((providers) => {
					scimProvider = providers.find(
						(provider) => provider.configured && provider.scimState === 'connected'
					);
				})
				.catch(() => {
					// Only the hint about SCIM setup depends on it.
				});
		}
	});

	async function handleBootstrapLogin() {
		try {
			await AdminService.bootstrapLogin(bootstrapToken);
			reloadPage();
		} catch (err) {
			error = err instanceof Error ? err.message : m.auth_bootstrap_unknown_error();
		}
	}
</script>

<div class="flex min-h-dvh flex-col items-center">
	<main
		class="bg-base-200 default-scrollbar-thin dark:bg-base-100 relative flex h-svh w-full grow flex-col overflow-y-auto"
	>
		<Navbar class="dark:bg-gray-990 sticky top-0 left-0 z-30 w-full" unauthorized />
		<div class="flex min-h-1 w-full grow items-center justify-center p-4 md:p-0">
			{#await fetchBootstrapStatus}
				<div class="size-10">
					<Loading class="size-8" />
				</div>
			{:then bootstrapStatus}
				{#if showSetupHandoff}
					{@render setupView()}
				{:else}
					{@render loginView(bootstrapStatus)}
				{/if}
			{/await}
		</div>
	</main>
</div>

{#snippet setupView()}
	{#await tempDataPromises}
		<div class="size-10">
			<Loading class="size-8" />
		</div>
	{:then response}
		{@const [tempUser, explicitRoles] = response ?? []}
		{@const isExplicitAdmin = explicitRoles?.admins?.includes(tempUser?.email ?? '') ?? false}
		{#if tempUser}
			<div
				class="dark:bg-base-300 dark:border-base-400 bg-base-100 flex w-md max-w-full flex-col rounded-lg border border-transparent px-4 py-8 shadow-sm"
			>
				<BetaLogo class="self-center" />

				{#if showSuccessOwnerConfirmation}
					<div class="my-6 flex w-full flex-col items-center justify-center gap-6">
						<p class="text-md px-4 text-left font-light">
							{m.auth_bootstrap_handoff_done()}
						</p>
						{#if scimProvider}
							<p class="text-md px-4 text-left font-light">
								{scimProvider.name} provisions users and groups through SCIM. After you log in, setup
								continues on Identity &amp; Access → Auth Providers → SCIM: generate the token, create
								the SCIM app in
								{scimProvider.name}, and assign users.
							</p>
						{/if}
					</div>
					<button
						class="btn btn-secondary place-items-center"
						onclick={async () => {
							await AdminService.bootstrapLogout();
							// make sure to clear seenSplashDialog so splash will show for logged in owner if needed
							localStorage.removeItem(SEEN_SPLASH_DIALOG_KEY);
							clearProductAnalyticsConsentDeferral();
							navigateTo(`/oauth2/sign_out?rd=${encodeURIComponent(afterSignOut)}`);
						}}
					>
						{m.profile_log_out()}
					</button>
				{:else}
					<div class="my-6 flex w-full flex-col items-center justify-center gap-6 px-8">
						<div class="flex items-center justify-center gap-2">
							{#if isExplicitAdmin}
								<ShieldAlert class="size-6" />
								<h3 class="text-xl font-semibold">{m.auth_bootstrap_explicit_admin_set()}</h3>
							{:else}
								<Handshake class="size-6" />
								<h3 class="text-xl font-semibold">{m.auth_bootstrap_confirm_new_owner()}</h3>
							{/if}
						</div>

						<p class="text-md text-center font-light">
							{m.auth_bootstrap_logged_in_as_prefix()}
							<span class="font-semibold">{tempUser.email || tempUser.username}</span
							>{m.auth_bootstrap_logged_in_as_suffix()}
						</p>

						<p class="text-md text-center font-light" class:text-left={isExplicitAdmin}>
							{#if isExplicitAdmin}
								{m.auth_bootstrap_explicit_admin_note_prefix()}
								<a
									class="text-link"
									target="_blank"
									rel="external noopener noreferrer"
									href="https://docs.obot.ai/configuration/auth-providers#preconfiguring-owner--admin-users"
									>{m.auth_bootstrap_explicit_admin_note_link()}</a
								>
								{m.auth_bootstrap_explicit_admin_note_suffix()}
							{:else}
								{m.auth_bootstrap_make_owner_confirm()}
							{/if}
						</p>
					</div>
					<div class="flex flex-col gap-2">
						{#if !isExplicitAdmin}
							<button
								class="btn btn-primary place-items-center"
								onclick={async () => {
									loadingConfirmTempUser = true;
									await AdminService.confirmTempUserAsOwner(tempUser.email);
									loadingConfirmTempUser = false;
									showSuccessOwnerConfirmation = true;
								}}
								disabled={loadingCancelTempUser || loadingConfirmTempUser}
							>
								{#if loadingConfirmTempUser}
									<Loading class="size-4" />
								{:else}
									{m.auth_bootstrap_make_owner_yes()}
								{/if}
							</button>
						{/if}
						<button
							class="btn btn-secondary place-items-center"
							onclick={async () => {
								loadingCancelTempUser = true;
								await AdminService.cancelTempLogin();
								goto('/identity-access?view=auth-providers', { replaceState: true });
							}}
							disabled={loadingCancelTempUser || loadingConfirmTempUser}
						>
							{#if loadingCancelTempUser}
								<Loading class="size-4" />
							{:else}
								{isExplicitAdmin ? m.auth_bootstrap_go_back() : m.auth_bootstrap_cancel_go_back()}
							{/if}
						</button>
					</div>
				{/if}
			</div>
		{/if}
	{/await}
{/snippet}

{#snippet loginView(bootstrapStatus?: BootstrapStatus)}
	<form
		class="dark:bg-base-300 dark:border-base-400 bg-base-100 flex w-sm flex-col rounded-lg border border-transparent px-4 py-8 shadow-sm"
		onsubmit={(e) => e.preventDefault()}
	>
		<BetaLogo class="self-center" />

		{#if error}
			<div class="notification-error mt-4 flex items-center gap-2">
				<CircleAlert class="size-6 text-error" />
				<p class="flex flex-col text-sm font-light">
					<span class="font-semibold">{m.auth_bootstrap_error_occurred()}</span>
					<span>
						{error}
					</span>
				</p>
			</div>
		{/if}

		{#if loggedIn && !hasAccess}
			<div class="relative z-10 my-6 flex w-full flex-col items-center justify-center gap-6">
				<p class="text-muted-content px-8 text-center text-sm font-light md:px-8">
					{m.auth_bootstrap_not_authorized()}
				</p>
			</div>

			<a
				href={resolve('/oauth2/sign_out?rd=/admin')}
				onclick={clearProductAnalyticsConsentDeferral}
				class="bg-base-200 hover:bg-base-300 dark:bg-base-200 dark:hover:bg-base-300 flex w-full items-center justify-center gap-1.5 rounded-full p-2 px-8 text-lg font-semibold"
			>
				<p class="text-center text-sm font-medium">{m.auth_bootstrap_sign_out()}</p>
			</a>
		{:else if authProviders.length > 0}
			<div class="relative z-10 mt-6 flex w-full flex-col items-center justify-center gap-6">
				<p class="text-md text-muted-content px-8 text-center font-light md:px-8">
					{m.auth_bootstrap_sign_in_prompt()}
				</p>
				<h3 class="dark:bg-base-300 bg-base-100 px-2 text-lg font-semibold">
					{m.auth_bootstrap_sign_in_heading()}
				</h3>
			</div>

			<div
				class="border-base-400 relative flex -translate-y-4 flex-col items-center gap-4 rounded-xl border-2 px-4 pt-6 pb-4"
			>
				{#each authProviders as authProvider (authProvider.id)}
					<button
						class="btn btn-secondary w-full"
						onclick={() => {
							localStorage.setItem('preAuthRedirect', window.location.href);
							window.location.href = `/oauth2/start?rd=${encodeURIComponent(
								'/admin'
							)}&obot-auth-provider=${authProvider.namespace}/${authProvider.id}`;
						}}
					>
						{#if authProvider.icon}
							<img
								class="h-6 w-6 rounded-full bg-base-100 p-1 dark:bg-gray-600"
								src={authProvider.icon}
								alt={authProvider.name}
							/>
							<span class="text-center text-sm font-light"
								>{m.login_continue_with({ provider: authProvider.name })}</span
							>
						{/if}
					</button>
				{/each}

				{#if !showBootstrapLogin && bootstrapStatus?.enabled}
					<button
						onclick={() => (showBootstrapLogin = true)}
						class="bg-base-200 hover:bg-base-300 dark:bg-base-200 dark:hover:bg-base-300 flex w-full items-center justify-center gap-1.5 rounded-full p-2 px-8 text-lg font-semibold"
					>
						<p class="text-center text-sm font-medium">
							{m.auth_bootstrap_sign_in_bootstrap()}
						</p>
					</button>
				{/if}
			</div>
		{/if}

		{#if showBootstrapLogin && bootstrapStatus?.enabled && !loggedIn}
			<div class="flex flex-col gap-4" in:slide class:mt-4={authProviders.length === 0}>
				<h4 class="text-center text-lg font-semibold">{m.auth_bootstrap()}</h4>
				<p class="text-md font-light">{m.auth_bootstrap_enter_bootstrap()}</p>

				<div class="text-md flex flex-col gap-1">
					<label for="bootstrap-token" class="font-semibold">{m.auth_bootstrap_token()}</label>
					<SensitiveInput name="bootstrap-token" bind:value={bootstrapToken} />
				</div>

				<i class="text-xs font-light">
					{m.auth_bootstrap_hint()}
				</i>

				<button class="btn btn-primary mt-4 text-sm" onclick={handleBootstrapLogin}>
					{m.auth_bootstrap_login_bootstrap()}
				</button>
			</div>
		{/if}
	</form>
{/snippet}

<svelte:head>
	<title>Obot | {m.auth_bootstrap_page_title()}</title>
</svelte:head>
