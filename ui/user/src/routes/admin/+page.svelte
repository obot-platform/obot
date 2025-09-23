<script lang="ts">
	import Navbar from '$lib/components/Navbar.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import { AdminService, type BootstrapStatus } from '$lib/services';
	import { darkMode } from '$lib/stores';
	import { AlertCircle, LoaderCircle } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { slide } from 'svelte/transition';

	const { data } = $props();
	const { authProviders, loggedIn, hasAccess } = data;
	let fetchBootstrapStatus = $state<Promise<BootstrapStatus>>();
	let bootstrapToken = $state('');
	let error = $state('');
	let showBootstrapLogin = $state(authProviders.length === 0);

	onMount(() => {
		fetchBootstrapStatus = AdminService.getBootstrapStatus();
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
		class="bg-surface1 default-scrollbar-thin relative flex h-svh w-full grow flex-col overflow-y-auto dark:bg-black"
	>
		<Navbar class="dark:bg-gray-990 sticky top-0 left-0 z-30 w-full" unauthorized />
		<div class="flex min-h-1 w-full grow items-center justify-center">
			{#await fetchBootstrapStatus}
				<div class="size-10">
					<LoaderCircle class="text-primary size-8 animate-spin" />
				</div>
			{:then bootstrapStatus}
				<form
					class="dark:bg-surface2 dark:border-surface3 flex w-sm flex-col rounded-lg border border-transparent bg-white px-4 py-8 shadow-sm"
					onsubmit={(e) => e.preventDefault()}
				>
					{#if darkMode.isDark}
						<img src="/user/images/obot-logo-blue-white-text.svg" class="h-12" alt="Obot logo" />
					{:else}
						<img src="/user/images/obot-logo-blue-black-text.svg" class="h-12" alt="Obot logo" />
					{/if}

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
							<p
								class="px-8 text-center text-sm font-light text-gray-500 md:px-8 dark:text-gray-300"
							>
								You are not authorized to access this page. Please sign in with an authorized
								account or contact your administrator.
							</p>
						</div>

						<a
							href="/oauth2/sign_out?rd=/admin"
							class="bg-surface1 hover:bg-surface2 dark:bg-surface1 dark:hover:bg-surface3 flex w-full items-center justify-center gap-1.5 rounded-full p-2 px-8 text-lg font-semibold"
						>
							<p class="text-center text-sm font-medium">Sign Out</p>
						</a>
					{:else if authProviders.length > 0}
						<div class="relative z-10 mt-6 flex w-full flex-col items-center justify-center gap-6">
							<p
								class="text-md px-8 text-center font-light text-gray-500 md:px-8 dark:text-gray-300"
							>
								To access the admin panel, you need to sign in with an option below.
							</p>
							<h3 class="dark:bg-surface2 bg-white px-2 text-lg font-semibold">
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
										<span class="text-center text-sm font-light"
											>Continue with {authProvider.name}</span
										>
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
								You can find the bootstrap token in the server logs when starting Obot by searching
								for 'Bootstrap Token', or configure it directly through environment variables at
								startup.
							</i>

							<button class="button-primary mt-4 text-sm" onclick={handleBootstrapLogin}>
								Login
							</button>
						</div>
					{/if}
				</form>
			{/await}
		</div>
	</main>
</div>

<svelte:head>
	<title>Obot | Admin</title>
</svelte:head>
