<script lang="ts">
	import { page } from '$app/state';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import LocalAuthConfigure from '$lib/components/admin/LocalAuthConfigure.svelte';
	import ProviderCard from '$lib/components/admin/ProviderCard.svelte';
	import ProviderConfigure from '$lib/components/admin/ProviderConfigure.svelte';
	import ProviderDeconfigureConfirm from '$lib/components/admin/ProviderDeconfigureConfirm.svelte';
	import LicenseProviderDialog from '$lib/components/admin/license/LicenseProviderDialog.svelte';
	import {
		CommonAuthProviderIds,
		PAGE_TRANSITION_DURATION,
		RecommendedModelProviders
	} from '$lib/constants';
	import { HttpError, parseErrorContent } from '$lib/errors.js';
	import { reloadPage } from '$lib/navigation';
	import { AdminService, UserService } from '$lib/services';
	import type { AuthProvider } from '$lib/services/admin/types.js';
	import { errors, license, profile, version } from '$lib/stores';
	import { adminConfigStore } from '$lib/stores/adminConfig.svelte.js';
	import { clearUrlParams } from '$lib/url';
	import { TriangleAlert, Info, CircleAlert } from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	let {
		authProviders: initialAuthProviders,
		authEnabled = true
	}: { authProviders: AuthProvider[]; authEnabled?: boolean } = $props();
	let authProviders = $state(untrack(() => initialAuthProviders));
	let licenseRequiredProvider = $state<AuthProvider>();

	function sortAuthProviders(authProviders: AuthProvider[]) {
		return [...authProviders].sort((a, b) => {
			if (a.id === CommonAuthProviderIds.LOCAL) return 1;
			if (b.id === CommonAuthProviderIds.LOCAL) return -1;

			const preferredOrder: string[] = [
				CommonAuthProviderIds.GOOGLE,
				CommonAuthProviderIds.GITHUB,
				CommonAuthProviderIds.OKTA,
				CommonAuthProviderIds.AUTH0
			];
			const aIndex = preferredOrder.indexOf(a.id);
			const bIndex = preferredOrder.indexOf(b.id);

			// If both providers are in preferredOrder, sort by their order
			if (aIndex !== -1 && bIndex !== -1) {
				return aIndex - bIndex;
			}

			// If only a is in preferredOrder, it comes first
			if (aIndex !== -1) return -1;
			// If only b is in preferredOrder, it comes first
			if (bIndex !== -1) return 1;

			// For all other providers, sort alphabetically by name
			return a.name.localeCompare(b.name);
		});
	}
	let sortedAuthProviders = $derived(sortAuthProviders(authProviders));
	let providerConfigure = $state<ReturnType<typeof ProviderConfigure>>();
	let configuringAuthProvider = $state<AuthProvider>();
	let configuringAuthProviderValues = $state<Record<string, string>>();
	let atLeastOneConfigured = $derived(authProviders.some((provider) => provider.configured));
	let showInitialAuthProvider = $derived<string | null>(page.url.searchParams.get('provider'));
	let stagedProvider = $derived(authProviders.find((provider) => provider.staged));
	let activeProvider = $derived(authProviders.find((provider) => provider.configured));
	let switchError = $state<string>();
	let switching = $state(false);

	// Set when the owner goes back to edit a staged provider's settings, which is how a rejected
	// sign-in gets fixed. Local intent rather than server state, so it overrides the derived step.
	let editingStagedCredentials = $state(false);
	// Which step of a switch the dialog shows. Every input is server state, so a refresh, a second
	// tab, and another administrator all see the same step, with nothing carried in the URL.
	let switchStep = $derived.by(() => {
		if (!configuringAuthProvider || editingStagedCredentials) return 'configure';
		const staged = authProviders.find((provider) => provider.id === configuringAuthProvider!.id);
		if (!staged?.staged) return 'configure';
		return staged.verifiedEmail ? 'switch' : 'signin';
	});
	let switchVerifiedEmail = $derived(
		authProviders.find((provider) => provider.id === configuringAuthProvider?.id)?.verifiedEmail
	);
	// A switch is only offered when this provider would replace a different one. Configuring the
	// first provider on a fresh install stays the plain form.
	let isOwner = $derived(!!profile.current.isOwner?.());
	let isReadonly = $derived(!!profile.current.isAdminReadonly?.());
	// Replacing the provider everyone signs in with is an owner operation, so an administrator is
	// not offered the switch at all rather than being stopped partway through it.
	function switchNeedsOwner(provider: AuthProvider) {
		return (
			!isReadonly &&
			!isOwner &&
			atLeastOneConfigured &&
			!provider.configured &&
			activeProvider?.id !== provider.id
		);
	}

	let isSwitching = $derived(
		!!configuringAuthProvider &&
			atLeastOneConfigured &&
			!configuringAuthProvider.configured &&
			activeProvider?.id !== configuringAuthProvider.id
	);

	let setupLoading = $state(false);
	let setupSignInDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let explicitOwners = $state<string[]>([]);
	let setupTempLoginUrl = $state('');

	let loading = $state(false);
	let configureError = $state<string>();

	let deconfigureAuthProviderDialog = $state<ReturnType<typeof ProviderDeconfigureConfirm>>();
	let confirmDeconfigureAuthProvider = $state<AuthProvider>();

	let localAuthConfigure = $state<ReturnType<typeof LocalAuthConfigure>>();
	let localAuthConfigureOpen = $state(false);

	let isBootstrapUser = $derived(profile.current.isBootstrapUser?.());

	const duration = PAGE_TRANSITION_DURATION;

	const prepareOwnerSetup = async () => {
		// Don't prompt for owner login while the local auth modal is open — the admin may still be
		// configuring it or adding the first user.
		if (localAuthConfigureOpen) return;

		const configuredAuthProvider = authProviders.find(
			(provider) => provider.configured && (provider.missingEntitlements || []).length === 0
		);
		if (!configuredAuthProvider) return;

		const bootstrapStatus = await UserService.getBootstrapStatus();
		if (!bootstrapStatus.setupEnabled) return;

		// Local auth has nobody to log in as until at least one user exists.
		if (configuredAuthProvider.id === CommonAuthProviderIds.LOCAL) {
			const localUsers = await AdminService.listLocalAuthUsers();
			if (localUsers.length === 0) return;
		}

		if (!setupLoading && !setupTempLoginUrl) {
			configuringAuthProvider = configuredAuthProvider;
			handleOwnerSetup();
		}
	};

	$effect(() => {
		if (!isBootstrapUser) return;

		prepareOwnerSetup();
	});

	$effect(() => {
		if (!isBootstrapUser) return;

		if (!atLeastOneConfigured) return;

		const handleVisibilityChange = async () => {
			if (document.visibilityState === 'visible') {
				prepareOwnerSetup();
			}
		};

		document.addEventListener('visibilitychange', handleVisibilityChange);

		return () => {
			document.removeEventListener('visibilitychange', handleVisibilityChange);
		};
	});

	$effect(() => {
		if (showInitialAuthProvider) {
			const authProvider = sortedAuthProviders.find(
				(provider) => provider.id === showInitialAuthProvider
			);
			if (authProvider) {
				handleClickConfigure(authProvider);
			}
		}
	});

	// Returning from the verification lands here with nothing in the URL to say so, and the switch
	// is then one click from done. Only the owner can finish it, so nobody else is interrupted by a
	// dialog they cannot act on. Guarded so closing it is respected for the rest of the page's life.
	let autoOpenedVerifiedSwitch = $state(false);
	$effect(() => {
		if (autoOpenedVerifiedSwitch || !isOwner || isBootstrapUser || localAuthConfigureOpen) return;

		const verified = authProviders.find((provider) => provider.staged && provider.verifiedEmail);
		if (!verified) return;

		autoOpenedVerifiedSwitch = true;
		handleClickConfigure(verified);
	});

	function getDocumentationUrl(authProviderId?: string) {
		if (!authProviderId) return undefined;
		const idRef = {
			[CommonAuthProviderIds.GOOGLE]: 'google',
			[CommonAuthProviderIds.GITHUB]: 'github',
			[CommonAuthProviderIds.OKTA]: 'okta-enterprise-only',
			[CommonAuthProviderIds.ENTRA]: 'entra-enterprise-only',
			[CommonAuthProviderIds.AUTH0]: 'auth0-enterprise-only',
			[CommonAuthProviderIds.JUMPCLOUD]: 'jumpcloud-enterprise-only'
		};
		return idRef[authProviderId as keyof typeof idRef]
			? `https://docs.obot.ai/configuration/auth-providers/#${idRef[authProviderId as keyof typeof idRef]}`
			: undefined;
	}

	async function handleOwnerSetup() {
		if (!configuringAuthProvider || setupLoading) return;

		setupLoading = true;

		try {
			await AdminService.cancelTempLogin();
		} catch (err) {
			if (err instanceof HttpError && err.statusCode === 404) {
				// ignore, no current temp login to cancel
			} else {
				errors.append(err);
			}
		}

		try {
			explicitOwners = (await AdminService.listExplicitRoleEmails())?.owners ?? [];
			setupTempLoginUrl = (
				await AdminService.initiateTempLogin(
					configuringAuthProvider.id,
					configuringAuthProvider.namespace
				)
			).redirectUrl;
			setupLoading = false;
			setupSignInDialog?.open();
		} catch (_) {
			// ignore
		}
	}

	async function handleAuthProviderConfigure(form: Record<string, string>) {
		if (configuringAuthProvider) {
			loading = true;
			configureError = undefined;
			try {
				const staging = isSwitching;
				if (staging) {
					await AdminService.stageAuthProvider(configuringAuthProvider.id, form);
				} else {
					await AdminService.configureAuthProvider(configuringAuthProvider.id, form);
				}
				authProviders = await AdminService.listAuthProviders();
				adminConfigStore.updateAuthProviders(authProviders);

				if (staging) {
					// Staging alone changes nothing about who serves logins, so the dialog stays open
					// and moves to the sign-in step rather than looking finished.
					switchError = undefined;
					editingStagedCredentials = false;
					return;
				}
				providerConfigure?.close();

				if (isBootstrapUser) {
					await handleOwnerSetup();
				}
			} catch (err: unknown) {
				configureError = parseErrorContent(err).message;
			} finally {
				loading = false;
			}
		}
	}

	// Saves the local auth provider's email-domain config. Returns an error message to show inside
	// the local auth modal, or undefined on success. The local provider manages its own users, so
	// unlike the OAuth providers it doesn't hand off to the owner-setup flow here — that happens
	// when the modal closes with at least one user.
	async function handleLocalAuthConfigure(
		form: Record<string, string>
	): Promise<string | undefined> {
		try {
			// Local follows the same rule as every other provider: with something else already
			// serving logins, saving settings stages a replacement rather than taking over.
			if (atLeastOneConfigured && activeProvider?.id !== CommonAuthProviderIds.LOCAL) {
				await AdminService.stageAuthProvider(CommonAuthProviderIds.LOCAL, form);
			} else {
				await AdminService.configureAuthProvider(CommonAuthProviderIds.LOCAL, form);
			}
			authProviders = await AdminService.listAuthProviders();
			adminConfigStore.updateAuthProviders(authProviders);
			return undefined;
		} catch (err) {
			return parseErrorContent(err).message;
		}
	}

	async function refreshAuthProviders() {
		authProviders = await AdminService.listAuthProviders();
		adminConfigStore.updateAuthProviders(authProviders);
	}

	async function handleVerifyStagedProvider() {
		if (!stagedProvider) return;
		switching = true;
		switchError = undefined;
		try {
			window.location.href = await AdminService.verifyAuthProvider(stagedProvider.id);
		} catch (err) {
			switchError = parseErrorContent(err).message;
			switching = false;
		}
	}

	async function handleActivateStagedProvider() {
		if (!stagedProvider) return;
		switching = true;
		switchError = undefined;
		try {
			await AdminService.activateAuthProvider(stagedProvider.id);
			providerConfigure?.close();
			await refreshAuthProviders();
		} catch (err) {
			switchError = parseErrorContent(err).message;
		} finally {
			switching = false;
		}
	}

	async function handleDiscardStagedProvider() {
		if (!stagedProvider) return;
		switching = true;
		switchError = undefined;
		try {
			await AdminService.unstageAuthProvider(stagedProvider.id);
			providerConfigure?.close();
			await refreshAuthProviders();
		} catch (err) {
			switchError = parseErrorContent(err).message;
		} finally {
			switching = false;
		}
	}

	async function handleDeconfigureAuthProvider() {
		if (!confirmDeconfigureAuthProvider) {
			console.error('No auth provider to deconfigure');
			return;
		}
		loading = true;
		try {
			await AdminService.deconfigureAuthProvider(confirmDeconfigureAuthProvider.id);
			if (isBootstrapUser) {
				reloadPage();
			} else {
				authProviders = await AdminService.listAuthProviders();
				adminConfigStore.updateAuthProviders(authProviders);
				if (authProviders.every((provider) => !provider.configured)) {
					// no auth provider set after deconfiguring, prompt relogin
					profile.current.expired = true;
				}
			}
		} catch (err) {
			errors.append(err);
		} finally {
			deconfigureAuthProviderDialog?.close();
			confirmDeconfigureAuthProvider = undefined;
			loading = false;
		}
	}

	async function handleCommunitySubmit() {
		if (!licenseRequiredProvider) return;

		const newVersion = await UserService.getVersion();
		version.initialize(newVersion);

		authProviders = await AdminService.listAuthProviders();
		adminConfigStore.updateAuthProviders(authProviders);

		const updatedMatch = authProviders.find(
			(provider) => provider.id === licenseRequiredProvider?.id
		);

		if (updatedMatch) {
			handleClickConfigure(updatedMatch);
		} else {
			errors.append('There was an issue fetching the auth provider configuration.');
		}

		licenseRequiredProvider = undefined;
	}

	async function handleClickConfigure(authProvider: AuthProvider) {
		if (authProvider.missingEntitlements && authProvider.missingEntitlements.length > 0) {
			licenseRequiredProvider = authProvider;
			return;
		}

		editingStagedCredentials = false;
		switchError = undefined;
		configuringAuthProvider = authProvider;
		try {
			configuringAuthProviderValues = await AdminService.revealAuthProvider(authProvider.id);
		} catch (err) {
			// if 404, ignore, it means no credentials are set
			if (!(err instanceof HttpError) || err.statusCode !== 404) {
				console.error('An error occurred while revealing auth provider credentials', err);
			} else {
				// no credentials set, set initial default value for allowed domains
				configuringAuthProviderValues = {
					OBOT_AUTH_PROVIDER_EMAIL_DOMAINS: '*'
				};
			}
		}

		// A staged Local provider is past the step its own dialog covers, so resuming goes to the
		// sign-in that proves it rather than back to editing users.
		if (authProvider.id === CommonAuthProviderIds.LOCAL && !authProvider.staged) {
			localAuthConfigureOpen = true;
			localAuthConfigure?.open();
		} else {
			providerConfigure?.open();
		}
	}

	// Local's first step lives in its own dialog, so going back from the sign-in step has to hand
	// control there instead of rendering a parameter form Local does not have.
	function handleEditStagedCredentials() {
		if (configuringAuthProvider?.id === CommonAuthProviderIds.LOCAL) {
			providerConfigure?.close();
			localAuthConfigureOpen = true;
			localAuthConfigure?.open();
			return;
		}
		editingStagedCredentials = true;
	}

	async function handleLocalAuthClose(userCount: number) {
		localAuthConfigureOpen = false;
		clearUrlParams(['provider']);
		showInitialAuthProvider = null;
		if (isBootstrapUser && userCount > 0) {
			await prepareOwnerSetup();
		}

		// Local manages its users in its own dialog, so that dialog stands in for the first step of
		// a switch. Without this handoff the settings stage and the flow stops there.
		await refreshAuthProviders();
		const local = authProviders.find(
			(provider) => provider.id === CommonAuthProviderIds.LOCAL && provider.staged
		);
		if (local && userCount > 0) {
			configuringAuthProvider = local;
			editingStagedCredentials = false;
			switchError = undefined;
			providerConfigure?.open();
		}
	}
</script>

<div class="mb-4 w-full" in:fade={{ duration }}>
	{#if authEnabled}
		<div class="flex flex-col gap-8">
			{#if !atLeastOneConfigured}
				<div class="notification-alert mb-4 flex flex-col gap-2">
					<div class="flex items-center gap-2">
						<TriangleAlert class="size-6 shrink-0 self-start text-warning" />
						<p class="my-0.5 flex flex-col text-sm font-semibold">No Auth Providers Configured!</p>
					</div>
					<span class="text-sm font-light break-all">
						To finish setting up Obot, you'll need to configure an Auth Provider. Select one below
						to get started!
					</span>
				</div>
			{/if}
		</div>
		<div class="grid grid-cols-1 gap-4 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
			{#each sortedAuthProviders as authProvider (authProvider.id)}
				<ProviderCard
					disableConfigure={switchNeedsOwner(authProvider) ||
						(atLeastOneConfigured &&
							!authProvider.configured &&
							!!stagedProvider &&
							!authProvider.staged)}
					disableConfigureReason={switchNeedsOwner(authProvider)
						? 'Only an owner can replace the auth provider everyone signs in with.'
						: undefined}
					provider={authProvider}
					staged={authProvider.staged}
					recommended={RecommendedModelProviders.includes(authProvider.id)}
					onConfigure={() => handleClickConfigure(authProvider)}
					onDeconfigure={activeProvider?.id === authProvider.id
						? undefined
						: () => {
								confirmDeconfigureAuthProvider = authProvider;
								deconfigureAuthProviderDialog?.open();
							}}
					readonly={isReadonly}
					licenseKey={license.current.licenseKey}
				/>
			{/each}
		</div>
	{:else}
		<p class="text-muted-content text-sm font-light">Authentication is not enabled.</p>
	{/if}
</div>

{#snippet switchSteps()}
	{@const done = { configure: 0, signin: 1, switch: 2 }[switchStep] ?? 0}
	<ol class="flex items-center gap-2 px-4 pb-4">
		{#each ['Configure', 'Sign in', 'Switch'] as label, index (label)}
			{#if index > 0}
				<li class="bg-base-400 h-px min-w-3 grow" aria-hidden="true"></li>
			{/if}
			<li class="flex flex-none items-center gap-1.5">
				<span
					class={twMerge(
						'flex size-5 items-center justify-center rounded-full border text-[11px]',
						index < done && 'border-success bg-success text-white',
						index === done && 'border-primary bg-primary text-white',
						index > done && 'border-base-400 text-muted-content'
					)}
					aria-current={index === done ? 'step' : undefined}
				>
					{#if index < done}✓{:else}{index + 1}{/if}
				</span>
				<span
					class={twMerge(
						'text-sm',
						index === done ? 'font-medium' : 'text-muted-content font-light'
					)}
				>
					{label}
				</span>
			</li>
		{/each}
	</ol>
{/snippet}

{#snippet switchBody()}
	{#if switchError}
		<div class="notification-error flex items-start gap-2" role="alert">
			<CircleAlert class="mt-0.5 size-5 shrink-0 text-error" />
			<p class="text-sm font-light">{switchError}</p>
		</div>
	{/if}
	{#if switchStep === 'signin'}
		<p class="text-sm font-light">
			Sign in with <b>{configuringAuthProvider?.name}</b> to confirm the connection works. The
			account you use <b>becomes the owner of Obot</b> after the switch.
		</p>
		<div class="notification-info p-3 text-sm font-light">
			{activeProvider?.name ?? 'The current provider'} keeps serving logins until you finish the switch.
			You can come back to this step later.
		</div>
	{:else}
		<div class="bg-base-200 flex items-center gap-3 rounded-lg p-3">
			<div class="flex min-w-0 flex-col">
				<span class="truncate text-sm font-medium">{switchVerifiedEmail}</span>
				<span class="text-muted-content text-xs font-light"> Will own Obot after the switch </span>
			</div>
			<span class="text-success ml-auto flex-none text-xs">Verified</span>
		</div>
		<p class="text-muted-content text-xs font-light">
			Not the right account?
			<button class="text-link underline" disabled={switching} onclick={handleVerifyStagedProvider}>
				Sign in again
			</button>
		</p>
		<div class="notification-alert flex items-start gap-2 text-sm font-light">
			<TriangleAlert class="mt-0.5 size-5 shrink-0 text-warning" />
			<span>
				{configuringAuthProvider?.name} becomes the only way to sign in, and
				{activeProvider?.name ?? 'the current provider'} sessions end. Its users and their work will not
				transfer.
			</span>
		</div>
	{/if}
{/snippet}

{#snippet switchFooter(submit: () => void)}
	{#if switchStep !== 'configure'}
		<button
			class="btn btn-link text-muted-content px-0"
			disabled={switching}
			onclick={handleDiscardStagedProvider}
		>
			Discard switch
		</button>
	{/if}
	<div class="grow"></div>
	{#if switchStep === 'configure'}
		<button class="btn" disabled={loading} onclick={() => providerConfigure?.close()}>
			Cancel
		</button>
		<button class="btn btn-primary" disabled={loading} onclick={submit}>Continue</button>
	{:else if switchStep === 'signin'}
		<button class="btn" disabled={switching} onclick={handleEditStagedCredentials}> Back </button>
		<button class="btn btn-primary" disabled={switching} onclick={handleVerifyStagedProvider}>
			Sign in with {configuringAuthProvider?.name}
		</button>
	{:else}
		<button class="btn btn-primary" disabled={switching} onclick={handleActivateStagedProvider}>
			Switch to {configuringAuthProvider?.name}
		</button>
	{/if}
{/snippet}

<ProviderConfigure
	bind:this={providerConfigure}
	provider={configuringAuthProvider}
	values={configuringAuthProviderValues}
	onConfigure={handleAuthProviderConfigure}
	{loading}
	error={configureError}
	readonly={profile.current.isAdminReadonly?.()}
	title={isSwitching ? `Switch to ${configuringAuthProvider?.name}` : undefined}
	steps={isSwitching ? switchSteps : undefined}
	body={isSwitching && switchStep !== 'configure' ? switchBody : undefined}
	footer={isSwitching ? switchFooter : undefined}
>
	{#snippet note()}
		{@const documentationUrl = getDocumentationUrl(configuringAuthProvider?.id)}
		{@const callbackUrl = window.location.protocol + '//' + window.location.host + '/'}
		<div class="notification-info p-3 text-sm font-light">
			<div class="flex items-center gap-3">
				<Info class="size-6" />
				<p class="flex flex-wrap items-center gap-2">
					Note: the callback URL for this auth provider is
					<CopyButton
						showTextLeft
						buttonText={callbackUrl}
						text={callbackUrl}
						classes={{
							button: 'group'
						}}
						class="group-hover:text-white"
					/>
				</p>
			</div>
		</div>
		{#if documentationUrl}
			<div class="notification-info p-3 text-xs font-light">
				For more details, please review <a
					class="text-link"
					href={documentationUrl}
					rel="external noopener noreferrer"
					target="_blank">the documentation</a
				> for configuring this auth provider.
			</div>
		{/if}
	{/snippet}
</ProviderConfigure>

<LocalAuthConfigure
	required={!!showInitialAuthProvider}
	animate={showInitialAuthProvider ? 'slide' : undefined}
	bind:this={localAuthConfigure}
	provider={configuringAuthProvider}
	values={configuringAuthProviderValues}
	readonly={profile.current.isAdminReadonly?.()}
	onConfigure={handleLocalAuthConfigure}
	onClose={handleLocalAuthClose}
	switching={atLeastOneConfigured && activeProvider?.id !== CommonAuthProviderIds.LOCAL}
>
	{#snippet additionalActions()}
		{#if showInitialAuthProvider}
			<button
				type="button"
				class="btn btn-secondary text-xs"
				onclick={async () => {
					localAuthConfigure?.close();
					await handleLocalAuthClose(0);
				}}
			>
				Choose different provider
			</button>
		{/if}
	{/snippet}
</LocalAuthConfigure>

<ProviderDeconfigureConfirm
	bind:this={deconfigureAuthProviderDialog}
	providers={confirmDeconfigureAuthProvider ? [confirmDeconfigureAuthProvider] : undefined}
	onConfirm={handleDeconfigureAuthProvider}
	onCancel={() => {
		deconfigureAuthProviderDialog?.close();
		confirmDeconfigureAuthProvider = undefined;
	}}
	{loading}
/>

<ResponsiveDialog bind:this={setupSignInDialog} class="w-md">
	{#snippet titleContent()}
		<h3 class="text-lg font-semibold">Next Step: Owner Login Setup</h3>
	{/snippet}

	<div class="flex flex-col gap-4">
		{#if explicitOwners.length > 0}
			<p>You'll need to continue setup with an owner account.</p>
			<p>The following user(s) have been explicitly assigned the Owner role:</p>
			<ul class="list-disc px-8">
				{#each explicitOwners as owner (owner)}
					<li>{owner}</li>
				{/each}
			</ul>
			<p>
				Log in into the system as one of the explicit owners -- you'll be redirected back to the
				admin panel after authenticating.
			</p>
			<p>
				Or log into a different account with your configured auth provider. After authentication,
				you'll be asked to confirm the owner addition before proceeding.
			</p>
		{:else}
			<p>
				You'll need to set up an initial owner for the system. Login with your configured auth
				provider to continue.
			</p>
		{/if}

		<div class="my-4 flex flex-col gap-2">
			<!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- external temp login URL -->
			<a class="btn btn-secondary w-full" href={setupTempLoginUrl} rel="external">
				{#if configuringAuthProvider?.icon}
					<img
						class="h-6 w-6 rounded-full bg-base-100 p-1 dark:bg-gray-600"
						src={configuringAuthProvider.icon}
						alt={configuringAuthProvider.name}
					/>
					<span class="text-center text-sm font-light">
						Continue with {configuringAuthProvider.name}
					</span>
				{/if}
			</a>
		</div>
	</div>
</ResponsiveDialog>

<LicenseProviderDialog
	bind:provider={licenseRequiredProvider}
	allowSignup={!licenseRequiredProvider?.configured}
	licenseKey={license.current.licenseKey}
	endpoint={AdminService.createCommunityLicense}
	onSubmit={handleCommunitySubmit}
	signUpMessage="Register to unlock all remaining providers and to subscribe to the free Obot Community Newsletter."
/>
