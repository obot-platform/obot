<script lang="ts">
	import { resolve } from '$app/paths';
	import Navbar from '$lib/components/Navbar.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import BetaLogo from '$lib/components/navbar/BetaLogo.svelte';
	import { AdminService, type BootstrapStatus, type TempUser } from '$lib/services';
	import { goto } from '$lib/url';
	import { AlertCircle, Handshake, LoaderCircle, ShieldAlert } from 'lucide-svelte';
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

	onMount(() => {
		fetchBootstrapStatus = AdminService.getBootstrapStatus();
		if (showSetupHandoff) {
			tempDataPromises = Promise.all([
				AdminService.getTempUser(),
				AdminService.listExplicitRoleEmails()
			]);
		}
	});

	async function handleBootstrapLogin() {
		try {
			await AdminService.bootstrapLogin(bootstrapToken);
			window.location.reload();
		} catch (err) {
			error = err instanceof Error ? err.message : 'An unknown error occurred';
		}
	}
</script>

<div class="flex min-h-dvh flex-col items-center">
	<main
		class="bg-surface1 default-scrollbar-thin dark:bg-background relative flex h-svh w-full grow flex-col overflow-y-auto"
	>
		<Navbar class="dark:bg-gray-990 sticky top-0 left-0 z-30 w-full" unauthorized />
		<div class="flex min-h-1 w-full grow items-center justify-center">
			{#await fetchBootstrapStatus}
				<div class="size-10">
					<LoaderCircle class="text-primary size-8 animate-spin" />
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
			<LoaderCircle class="text-primary size-8 animate-spin" />
		</div>
	{:then response}
		{@const [tempUser, explicitRoles] = response ?? []}
		{@const isExplicitAdmin = explicitRoles?.admins?.includes(tempUser?.email ?? '') ?? false}
		{#if tempUser}
			<div
				class="dark:bg-surface2 dark:border-surface3 bg-background flex w-md max-w-full flex-col rounded-lg border border-transparent px-4 py-8 shadow-sm"
			>
				<BetaLogo class="self-center" />

				{#if showSuccessOwnerConfirmation}
					<div class="my-6 flex w-full flex-col items-center justify-center gap-6">
						<div class="flex items-center justify-center gap-2">
							<Handshake class="size-6" />
							<h3 class="text-xl font-semibold">Confirm Handoff</h3>
						</div>
						<p class="text-md px-4 text-left font-light">
							You've established your first owner user, the bootstrap user currently being used will
							be disabled. Upon completing this action, you'll be logged out and asked to log in
							using your auth provider.
						</p>
					</div>
					<button
						class="button place-items-center"
						onclick={async () => {
							await AdminService.bootstrapLogout();
							// make sure to clear seenSplashDialog so splash will show for logged in owner if needed
							localStorage.removeItem('seenSplashDialog');
							window.location.href = '/oauth2/sign_out?rd=/admin';
						}}
					>
						Confirm & Log Out
					</button>
				{:else}
					<div class="my-6 flex w-full flex-col items-center justify-center gap-6 px-8">
						<div class="flex items-center justify-center gap-2">
							{#if isExplicitAdmin}
								<ShieldAlert class="size-6" />
								<h3 class="text-xl font-semibold">Explicit Admin Already Set</h3>
							{:else}
								<Handshake class="size-6" />
								<h3 class="text-xl font-semibold">Confirm Owner Addition</h3>
							{/if}
						</div>

						<p class="text-md text-center font-light">
							You're now logged in as <span class="font-semibold"
								>{tempUser.email || tempUser.username}</span
							>.
						</p>

						<p class="text-md text-center font-light" class:text-left={isExplicitAdmin}>
							{#if isExplicitAdmin}
								This account has been explicitly assigned the Admin role. It cannot be modified. Go
								back and assign the Owner role to another account or adjust the preconfiguration to
								set this account as an owner instead. (See <a
									class="text-link"
									target="_blank"
									rel="external"
									href="https://docs.obot.ai/configuration/auth-providers#preconfiguring-owner--admin-users"
									>Preconfiguring Owner & Admin Users</a
								> for more information.)
							{:else}
								Are you sure you wish to make this account an owner?
							{/if}
						</p>
					</div>
					<div class="flex flex-col gap-2">
						{#if !isExplicitAdmin}
							<button
								class="button-primary place-items-center"
								onclick={async () => {
									loadingConfirmTempUser = true;
									await AdminService.confirmTempUserAsOwner(tempUser.email);
									loadingConfirmTempUser = false;
									showSuccessOwnerConfirmation = true;
								}}
								disabled={loadingCancelTempUser || loadingConfirmTempUser}
							>
								{#if loadingConfirmTempUser}
									<LoaderCircle class="size-4 animate-spin" />
								{:else}
									Yes, make this account an owner
								{/if}
							</button>
						{/if}
						<button
							class="button place-items-center"
							onclick={async () => {
								loadingCancelTempUser = true;
								await AdminService.cancelTempLogin();
								goto('/admin/auth-providers', { replaceState: true });
							}}
							disabled={loadingCancelTempUser || loadingConfirmTempUser}
						>
							{#if loadingCancelTempUser}
								<LoaderCircle class="size-4 animate-spin" />
							{:else}
								{isExplicitAdmin ? 'Go Back' : 'No, cancel & go back'}
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
		class="dark:bg-surface2 dark:border-surface3 bg-background flex w-sm flex-col rounded-lg border border-transparent px-4 py-8 shadow-sm"
		onsubmit={(e) => e.preventDefault()}
	>
		<BetaLogo class="self-center" />

		{#if error}
			<div class="notification-error mt-4 flex items-center gap-2">
				<AlertCircle class="size-6 text-red-500" />
				<p class="flex flex-col text-sm font-light">
					<span class="font-semibold">An error occurred!</span>
					<span>
						{error}
					</span>
				</p>
			</div>
		{/if}

		{#if loggedIn && !hasAccess}
			<div class="relative z-10 my-6 flex w-full flex-col items-center justify-center gap-6">
				<p class="text-on-surface1 px-8 text-center text-sm font-light md:px-8">
					You are not authorized to access this page. Please sign in with an authorized account or
					contact your administrator.
				</p>
			</div>

			<a
				href={resolve('/oauth2/sign_out?rd=/admin')}
				class="bg-surface1 hover:bg-surface2 dark:bg-surface1 dark:hover:bg-surface3 flex w-full items-center justify-center gap-1.5 rounded-full p-2 px-8 text-lg font-semibold"
			>
				<p class="text-center text-sm font-medium">Sign Out</p>
			</a>
		{:else if authProviders.length > 0}
			<div class="relative z-10 mt-6 flex w-full flex-col items-center justify-center gap-6">
				<p class="text-md text-on-surface1 px-8 text-center font-light md:px-8">
					To access the admin panel, you need to sign in with an option below.
				</p>
				<h3 class="dark:bg-surface2 bg-background px-2 text-lg font-semibold">
					Sign in to Your Account
				</h3>
			</div>

			<div
				class="border-surface3 relative flex -translate-y-4 flex-col items-center gap-4 rounded-xl border-2 px-4 pt-6 pb-4"
			>
				{#each authProviders as authProvider (authProvider.id)}
					<button
						class="group bg-surface1 hover:bg-surface2 dark:bg-surface1 dark:hover:bg-surface3 flex w-full items-center justify-center gap-1.5 rounded-full p-2 px-8 text-lg font-semibold"
						onclick={() => {
							localStorage.setItem('preAuthRedirect', window.location.href);
							window.location.href = `/oauth2/start?rd=${encodeURIComponent(
								'/admin'
							)}&obot-auth-provider=${authProvider.namespace}/${authProvider.id}`;
						}}
					>
						{#if authProvider.icon}
							<img
								class="h-6 w-6 rounded-full bg-transparent p-1 dark:bg-gray-600"
								src={authProvider.icon}
								alt={authProvider.name}
							/>
							<span class="text-center text-sm font-light">Continue with {authProvider.name}</span>
						{/if}
					</button>
				{/each}

				{#if !showBootstrapLogin && bootstrapStatus?.enabled}
					<button
						onclick={() => (showBootstrapLogin = true)}
						class="bg-surface1 hover:bg-surface2 dark:bg-surface1 dark:hover:bg-surface3 flex w-full items-center justify-center gap-1.5 rounded-full p-2 px-8 text-lg font-semibold"
					>
						<p class="text-center text-sm font-medium">Sign in with Bootstrap Token</p>
					</button>
				{/if}
			</div>
		{/if}

		{#if showBootstrapLogin && bootstrapStatus?.enabled && !loggedIn}
			<div class="flex flex-col gap-4" in:slide class:mt-4={authProviders.length === 0}>
				<h4 class="text-center text-lg font-semibold">Authenticate with Bootstrap Token</h4>
				<p class="text-md font-light">
					If this is your first time logging in, you will need to provide a bootstrap token.
				</p>

				<div class="text-md flex flex-col gap-1">
					<label for="bootstrap-token" class="font-semibold">Bootstrap Token</label>
					<SensitiveInput name="bootstrap-token" bind:value={bootstrapToken} />
				</div>

				<i class="text-xs font-light">
					You can find the bootstrap token in the server logs when starting Obot by searching for
					'Bootstrap Token', or configure it directly through environment variables at startup.
				</i>

				<button class="button-primary mt-4 text-sm" onclick={handleBootstrapLogin}> Login </button>
			</div>
		{/if}
	</form>
{/snippet}

<svelte:head>
	<title>Obot | Admin</title>
</svelte:head>
