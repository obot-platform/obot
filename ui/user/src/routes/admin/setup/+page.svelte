<script lang="ts">
	import Navbar from '$lib/components/Navbar.svelte';
	import LocalAuthConfigure from '$lib/components/admin/LocalAuthConfigure.svelte';
	import LocalAuthInitialUserForm from '$lib/components/admin/LocalAuthInitialUserForm.svelte';
	import OwnerSetupPrompt from '$lib/components/admin/OwnerSetupPrompt.svelte';
	import SetupSplashContent from '$lib/components/admin/SetupSplashContent.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { parseErrorContent } from '$lib/errors';
	import { AdminService, type AuthProvider, type LocalAuthUser } from '$lib/services';
	import { errors, version } from '$lib/stores';
	import { adminConfigStore } from '$lib/stores/adminConfig.svelte';
	import { goto } from '$lib/url';
	import type { PageData } from './$types';
	import { onMount, untrack } from 'svelte';
	import { fly } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	let { data }: { data: PageData } = $props();

	type SetupStep = 'splash' | 'owner' | 'signin';

	function initialStep(pageData: PageData): SetupStep {
		if (!pageData.splashSeen || !pageData.eulaAccepted) return 'splash';
		if (pageData.localUsers.length === 0) return 'owner';
		return 'signin';
	}

	let step = $state<SetupStep>(untrack(() => initialStep(data)));
	let localProvider = $state<AuthProvider | undefined>(untrack(() => data.localProvider));
	let localUsers = $state<LocalAuthUser[]>(untrack(() => data.localUsers));
	let explicitOwners = $state<string[]>([]);
	let setupTempLoginUrl = $state('');
	let reduceMotion = $state(false);
	let localAuthConfigure = $state<ReturnType<typeof LocalAuthConfigure>>();

	const duration = $derived(reduceMotion ? 0 : PAGE_TRANSITION_DURATION);
	const setupLocalUserEmail = $derived(localUsers.length === 1 ? localUsers[0].email : undefined);

	const cardClass =
		'dark:border-base-400 dark:bg-base-200 bg-base-100 flex w-full flex-col rounded-xl border border-transparent p-6 shadow-sm';

	onMount(() => {
		reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
		adminConfigStore.initialize();
		if (step === 'signin') {
			void prepareSignIn();
		}
	});

	async function handleLocalAuthConfigure(
		form: Record<string, string>
	): Promise<string | undefined> {
		if (!localProvider) return 'Local auth provider is not available.';
		try {
			await AdminService.configureAuthProvider(localProvider.id, form);
			const authProviders = await AdminService.listAuthProviders();
			adminConfigStore.updateAuthProviders(authProviders);
			localProvider = authProviders.find((provider) => provider.id === localProvider?.id);
			return undefined;
		} catch (err) {
			return parseErrorContent(err).message;
		}
	}

	async function startOwnerTempLogin(provider: AuthProvider) {
		try {
			await AdminService.cancelTempLogin();
		} catch {
			// Best-effort. 404 means nothing to cancel; other failures should not block setup.
		}

		const explicit = await AdminService.listExplicitRoleEmails();
		const { redirectUrl } = await AdminService.initiateTempLogin(provider.id, provider.namespace);

		return {
			explicitOwners: explicit?.owners ?? [],
			redirectUrl
		};
	}

	async function prepareSignIn() {
		if (!localProvider) return;
		try {
			const result = await startOwnerTempLogin(localProvider);
			explicitOwners = result.explicitOwners;
			setupTempLoginUrl = result.redirectUrl;
		} catch (err) {
			errors.append(err);
		}
	}

	async function handleSplashContinue() {
		if (!version.current.authEnabled) {
			goto('/dashboard');
			return;
		}
		if (localUsers.length > 0) {
			step = 'signin';
			await prepareSignIn();
			return;
		}
		step = 'owner';
	}

	async function handleOwnerCreated(userCount: number, email?: string) {
		if (userCount <= 0) return;
		if (email) {
			localUsers = [
				{
					id: 'created',
					email,
					created: new Date().toISOString(),
					requirePasswordChange: false
				}
			];
		} else {
			localUsers = await AdminService.listLocalAuthUsers();
		}
		step = 'signin';
		await prepareSignIn();
	}

	async function handleManageLocalUsers() {
		localAuthConfigure?.open();
	}

	async function handleLocalAuthClose(userCount: number) {
		localUsers = await AdminService.listLocalAuthUsers();
		if (userCount === 0) {
			step = 'owner';
		} else {
			await prepareSignIn();
		}
	}
</script>

<svelte:head>
	<title>Obot | Setup</title>
</svelte:head>

<div class="flex min-h-dvh flex-col items-center">
	<main
		class="text-base-content dark:from-base-300 to-base-200 flex h-dvh w-full flex-col items-center justify-center bg-radial-[at_50%_50%] from-gray-50 dark:to-black overflow-x-hidden px-4 default-scrollbar-thin overflow-y-auto"
	>
		<Navbar class="dark:bg-gray-990 sticky top-0 left-0 z-30 w-full" unauthorized />
		<div class="flex w-full grow items-center justify-center p-4 md:p-0">
			<div class="grid w-full max-w-xl place-items-center">
				{#if step === 'splash'}
					<div
						class={twMerge(cardClass, 'md:w-sm col-start-1 row-start-1')}
						in:fly={{ x: 80, duration }}
						out:fly={{ x: -80, duration }}
					>
						<SetupSplashContent onContinue={handleSplashContinue} />
					</div>
				{:else if step === 'owner'}
					<div
						class={twMerge(cardClass, 'md:w-xl col-start-1 row-start-1')}
						in:fly={{ x: 80, duration }}
						out:fly={{ x: -80, duration }}
					>
						<LocalAuthInitialUserForm
							provider={localProvider}
							onConfigure={handleLocalAuthConfigure}
							onCreated={handleOwnerCreated}
						/>
					</div>
				{:else}
					<div
						class={twMerge(cardClass, 'md:w-md col-start-1 row-start-1')}
						in:fly={{ x: 80, duration }}
						out:fly={{ x: -80, duration }}
					>
						<OwnerSetupPrompt
							isLocalSetup
							localUserEmail={setupLocalUserEmail}
							{explicitOwners}
							provider={localProvider}
							tempLoginUrl={setupTempLoginUrl}
							onManageLocalUsers={handleManageLocalUsers}
						/>
					</div>
				{/if}
			</div>
		</div>
	</main>
</div>

<LocalAuthConfigure
	bind:this={localAuthConfigure}
	provider={localProvider}
	values={{ OBOT_AUTH_PROVIDER_EMAIL_DOMAINS: '*' }}
	onConfigure={handleLocalAuthConfigure}
	bootstrap
	onClose={handleLocalAuthClose}
/>
