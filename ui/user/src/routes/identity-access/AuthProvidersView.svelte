<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import LocalAuthConfigure from '$lib/components/admin/LocalAuthConfigure.svelte';
	import OwnerSetupPrompt from '$lib/components/admin/OwnerSetupPrompt.svelte';
	import ProviderCard from '$lib/components/admin/ProviderCard.svelte';
	import ProviderConfigure, {
		type ParameterNotice
	} from '$lib/components/admin/ProviderConfigure.svelte';
	import ProviderDeconfigureConfirm from '$lib/components/admin/ProviderDeconfigureConfirm.svelte';
	import LicenseProviderDialog from '$lib/components/admin/license/LicenseProviderDialog.svelte';
	import { describeGroupReference } from '$lib/components/admin/scim/groupReferences';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import {
		CommonAuthProviderIds,
		PAGE_TRANSITION_DURATION,
		RecommendedModelProviders,
		SCIM_VIEW_PATH
	} from '$lib/constants';
	import { HttpError, parseErrorContent } from '$lib/errors.js';
	import { m } from '$lib/i18n';
	import { reloadPage } from '$lib/navigation';
	import { AdminService, UserService } from '$lib/services';
	import type {
		AuthProvider,
		ProviderParameter,
		ResidualGroupData
	} from '$lib/services/admin/types.js';
	import { errors, license, profile, version } from '$lib/stores';
	import { adminConfigStore } from '$lib/stores/adminConfig.svelte.js';
	import { clearUrlParams } from '$lib/url';
	import { TriangleAlert, Info, CircleAlert, ArrowLeft, Trash2 } from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { fade } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	let {
		authProviders: initialAuthProviders,
		authEnabled = true
	}: { authProviders: AuthProvider[]; authEnabled?: boolean } = $props();
	let authProviders = $state(untrack(() => initialAuthProviders));
	let licenseRequiredProvider = $state<AuthProvider>();

	const SWITCH_STEPS = ['configure', 'signin', 'switch'] as const;
	type SwitchStep = (typeof SWITCH_STEPS)[number];
	const SWITCH_STEP_LABELS: Record<SwitchStep, string> = {
		configure: m.identity_access_auth_providers_step_configure(),
		signin: m.identity_access_auth_providers_step_signin(),
		switch: m.identity_access_auth_providers_step_switch()
	};

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

	let switchVerifiedEmail = $derived(
		authProviders.find((provider) => provider.id === configuringAuthProvider?.id)?.verifiedEmail
	);
	let configurationLocked = $derived(!!switchVerifiedEmail);
	let signedInAsVerifiedAccount = $derived(
		!!switchVerifiedEmail && profile.current.currentAuthProvider === configuringAuthProvider?.id
	);
	let steppedBackTo = $state<SwitchStep>();
	// Which step of a switch the dialog shows. Every input is server state, so a refresh, a second
	// tab, and another administrator all see the same step, with nothing carried in the URL.
	let switchStep = $derived.by<SwitchStep>(() => {
		if (!configuringAuthProvider) return 'configure';
		const staged = authProviders.find((provider) => provider.id === configuringAuthProvider!.id);
		if (!staged?.staged) return 'configure';
		const reached: SwitchStep = staged.verifiedEmail ? 'switch' : 'signin';
		if (steppedBackTo === 'configure' && configurationLocked) return reached;
		if (steppedBackTo && SWITCH_STEPS.indexOf(steppedBackTo) < SWITCH_STEPS.indexOf(reached)) {
			return steppedBackTo;
		}
		return reached;
	});
	let confirmDiscardSwitch = $state(false);
	let confirmSwitch = $state(false);
	// Switching away from a provider that SCIM manages deletes its SCIM data, so a switch says so,
	// and what setting SCIM up for the incoming provider takes.
	let switchNote = $derived.by(() => {
		const incoming = configuringAuthProvider?.name;
		const notes = [
			m.identity_access_auth_providers_switch_cannot_undo({
				name: `${configuringAuthProvider?.name}`
			})
		];
		if (activeProvider?.scimState) {
			const outgoing = activeProvider.name;
			notes.push(m.identity_access_auth_providers_scim_switch_deletes({ name: outgoing }));
		}
		if (configuringAuthProvider?.scimState) {
			notes.push(m.identity_access_auth_providers_scim_switch_incoming({ name: incoming ?? '' }));
		}
		return notes.join(' ');
	});
	// Set once a switch completes to a provider that provisions through SCIM, whose setup is
	// finished on the SCIM tab. The layout's banner covers unfinished SCIM setup too.
	let scimNotice = $state<string>();
	// The provider whose configuration was refused for group data left from an earlier
	// configuration, which its auth provider cleanup removes.
	let residualProvider = $state<AuthProvider>();
	let residualData = $state<ResidualGroupData>();
	let residualCleanupStarted = $state(false);
	let residualCleanupLoading = $state(false);
	let confirmResidualCleanup = $state(false);
	// The cleanup removes admin-authored role assignments and policy subjects along with the groups, so the
	// confirmation names the groups that something still references.
	let referencedResidualGroups = $derived(
		residualData?.groups.filter((group) => group.references?.length) ?? []
	);
	let residualCleanupSummary = $derived.by(() => {
		// A referenced group ID that no group has is listed for its references, which go, but is no group.
		const groups = residualData?.groups.filter((group) => group.name).length ?? 0;
		const memberships = residualData?.membershipCount ?? 0;
		const groupLabel =
			groups === 1
				? m.identity_access_scim_count_group_one({ count: groups })
				: m.identity_access_scim_count_group_other({ count: groups });
		const membershipLabel =
			memberships === 1
				? m.identity_access_scim_count_membership_one({ count: memberships })
				: m.identity_access_scim_count_membership_other({ count: memberships });
		return m.identity_access_auth_providers_scim_residual_summary({
			groups: groupLabel,
			memberships: membershipLabel
		});
	});
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
	let setupLocalUserEmail = $state<string>();
	let isLocalSetup = $derived(configuringAuthProvider?.id === CommonAuthProviderIds.LOCAL);

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

			setupLocalUserEmail = localUsers.length === 1 ? localUsers[0].email : undefined;
		} else {
			setupLocalUserEmail = undefined;
		}

		if (!setupLoading && !setupTempLoginUrl) {
			configuringAuthProvider = configuredAuthProvider;
			await handleOwnerSetup();
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

	let autoOpenedInitialAuthProvider = $state(false);
	$effect(() => {
		if (autoOpenedInitialAuthProvider || !showInitialAuthProvider) return;

		const authProvider = sortedAuthProviders.find(
			(provider) => provider.id === showInitialAuthProvider
		);
		if (!authProvider) return;

		autoOpenedInitialAuthProvider = true;
		handleClickConfigure(authProvider);
	});

	// Reopens the switch dialog for a staged provider without waiting for a click, so an owner who
	// left mid-switch or is returning from the verification sign-in lands back in it. Owners only,
	// since the switch routes are owner-only. Guarded so closing it sticks for the rest of the page's
	// life.
	let autoOpenedStagedSwitch = $state(false);
	$effect(() => {
		if (autoOpenedStagedSwitch || !isOwner || isBootstrapUser || localAuthConfigureOpen) return;
		if (!stagedProvider) return;

		autoOpenedStagedSwitch = true;
		handleClickConfigure(stagedProvider);
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
			setupSignInDialog?.open();
		} catch (err) {
			errors.append(err);
		} finally {
			setupLoading = false;
		}
	}

	// Explains the choice that the directory parameters make while a provider that supports SCIM is
	// set up, and warns when the Org URL of a provider with a SCIM connection changes.
	function scimParameterNotice(
		parameter: ProviderParameter,
		form: Record<string, string>
	): ParameterNotice | undefined {
		const provider = configuringAuthProvider;
		const scim = provider?.scim;
		if (!provider || !scim) return undefined;

		if (parameter.name === scim.issuerParameter && provider.scimState && scim.connectionIssuer) {
			const value = (form[parameter.name] ?? '').trim().replace(/\/+$/, '');
			if (value && value !== scim.connectionIssuer) {
				return {
					kind: 'warning',
					text: m.identity_access_auth_providers_scim_issuer_warning({
						name: provider.name,
						issuer: scim.connectionIssuer
					})
				};
			}
		}

		const lastDirectoryParameter = scim.directoryParameters[scim.directoryParameters.length - 1];
		if (parameter.name === lastDirectoryParameter && !provider.scimState && !provider.configured) {
			const empty = scim.directoryParameters.every((name) => !form[name]?.trim());
			return {
				kind: 'info',
				text: empty
					? m.identity_access_auth_providers_scim_directory_empty({ name: provider.name })
					: m.identity_access_auth_providers_scim_directory_provided({ name: provider.name })
			};
		}
		return undefined;
	}

	async function handleRemoveResidualGroupData() {
		if (!residualProvider) return;
		residualCleanupLoading = true;
		try {
			await AdminService.deconfigureAuthProvider(residualProvider.id);
			residualCleanupStarted = true;
			configureError = undefined;
		} catch (err) {
			configureError = parseErrorContent(err).message;
		} finally {
			residualCleanupLoading = false;
			confirmResidualCleanup = false;
		}
	}

	function clearResidualGroupData() {
		residualProvider = undefined;
		residualData = undefined;
		residualCleanupStarted = false;
		confirmResidualCleanup = false;
	}

	async function handleAuthProviderConfigure(form: Record<string, string>) {
		if (configuringAuthProvider) {
			loading = true;
			configureError = undefined;
			clearResidualGroupData();
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
					steppedBackTo = undefined;
					return;
				}
				providerConfigure?.close();

				if (isBootstrapUser) {
					await handleOwnerSetup();
				}
			} catch (err: unknown) {
				configureError = parseErrorContent(err).message;
				// Group data left from an earlier configuration blocks setting a provider up for SCIM
				// until its auth provider cleanup, which deconfiguring runs, removes it.
				if (
					err instanceof HttpError &&
					err.statusCode === 409 &&
					configuringAuthProvider.scim &&
					!configuringAuthProvider.scimState
				) {
					const provider = configuringAuthProvider;
					try {
						const residual = await AdminService.getResidualGroupData(provider.id);
						if (residual.groups.length > 0 || residual.membershipCount > 0) {
							residualProvider = provider;
							residualData = residual;
						}
					} catch {
						// The refusal itself still explains what remains.
					}
				}
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
			const incoming = stagedProvider;
			await AdminService.activateAuthProvider(stagedProvider.id);
			confirmSwitch = false;
			providerConfigure?.close();
			await refreshAuthProviders();
			if (incoming.scimState) {
				scimNotice = m.identity_access_auth_providers_scim_notice({ name: incoming.name });
			}
		} catch (err) {
			confirmSwitch = false;
			switchError = parseErrorContent(err).message;
		} finally {
			switching = false;
		}
	}

	async function handleUnstageProvider() {
		if (!stagedProvider) return;
		switching = true;
		switchError = undefined;
		try {
			await AdminService.unstageAuthProvider(stagedProvider.id);
			confirmDiscardSwitch = false;
			if (signedInAsVerifiedAccount) {
				reloadPage();
				return;
			}
			providerConfigure?.close();
			await refreshAuthProviders();
		} catch (err) {
			confirmDiscardSwitch = false;
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
			errors.append(m.identity_access_auth_providers_fetch_config_failed());
		}

		licenseRequiredProvider = undefined;
	}

	async function handleClickConfigure(authProvider: AuthProvider) {
		if (authProvider.missingEntitlements && authProvider.missingEntitlements.length > 0) {
			licenseRequiredProvider = authProvider;
			return;
		}

		steppedBackTo = undefined;
		confirmDiscardSwitch = false;
		confirmSwitch = false;
		switchError = undefined;
		// A refusal for leftover group data, and a cleanup started for it, describe an earlier attempt.
		configureError = undefined;
		clearResidualGroupData();
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
			setupTempLoginUrl = '';
			localAuthConfigureOpen = true;
			localAuthConfigure?.open();
		} else {
			providerConfigure?.open();
		}
	}

	function handleManageLocalUsers() {
		const local = authProviders.find((provider) => provider.id === CommonAuthProviderIds.LOCAL);
		if (!local) return;

		setupSignInDialog?.close();
		handleClickConfigure(local);
	}

	// Local's first step lives in its own dialog, so going back from the sign-in step has to hand
	// control there instead of rendering a parameter form Local does not have.
	function goToSwitchStep(step: SwitchStep) {
		if (step === 'configure' && configurationLocked) return;

		switchError = undefined;
		if (step === 'configure' && configuringAuthProvider?.id === CommonAuthProviderIds.LOCAL) {
			providerConfigure?.close();
			localAuthConfigureOpen = true;
			localAuthConfigure?.open();
			return;
		}
		steppedBackTo = step;
	}

	async function handleLocalAuthClose(userCount: number) {
		localAuthConfigureOpen = false;
		autoOpenedInitialAuthProvider = false;
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
			steppedBackTo = undefined;
			switchError = undefined;
			providerConfigure?.open();
		}
	}
</script>

<div class="mb-4 w-full" in:fade={{ duration }}>
	{#if authEnabled}
		<div class="flex flex-col gap-8">
			{#if scimNotice}
				<div class="notification-info mb-4 flex items-start gap-2" role="status">
					<Info class="mt-0.5 size-5 shrink-0" />
					<p class="text-sm font-light">
						{scimNotice}
						<a class="text-link" href={resolve(SCIM_VIEW_PATH)}>{m.identity_access_scim_go_to()}</a>
					</p>
				</div>
			{/if}
			{#if !atLeastOneConfigured}
				<div class="notification-alert mb-4 flex flex-col gap-2">
					<div class="flex items-center gap-2">
						<TriangleAlert class="size-6 shrink-0 self-start text-warning" />
						<p class="my-0.5 flex flex-col text-sm font-semibold">
							{m.identity_access_auth_providers_none_configured()}
						</p>
					</div>
					<span class="text-sm font-light break-all">
						{m.identity_access_auth_providers_none_configured_description()}
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
						? m.identity_access_auth_providers_only_owner_can_replace()
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
		<p class="text-muted-content text-sm font-light">
			{m.identity_access_auth_providers_auth_not_enabled()}
		</p>
	{/if}
</div>

{#snippet switchSteps()}
	{@const done = SWITCH_STEPS.indexOf(switchStep)}
	<ol class="flex items-center gap-2 px-4 pb-4 mt-4 md:mt-0">
		{#each SWITCH_STEPS as step, index (step)}
			{@const label = SWITCH_STEP_LABELS[step]}
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
			{m.identity_access_auth_providers_signin_prefix()}<b>{configuringAuthProvider?.name}</b
			>{m.identity_access_auth_providers_signin_mid()}<b
				>{m.identity_access_auth_providers_signin_bold()}</b
			>{m.identity_access_auth_providers_signin_suffix()}
		</p>
		<div class="notification-info p-3 text-sm font-light">
			{m.identity_access_auth_providers_keeps_serving({
				provider:
					activeProvider?.name ?? m.identity_access_auth_providers_current_provider_capitalized()
			})}
		</div>
	{:else}
		<div class="bg-base-200 flex items-center gap-3 rounded-lg p-3">
			<div class="flex min-w-0 flex-col">
				<span class="truncate text-sm font-medium">{switchVerifiedEmail}</span>
				<span class="text-muted-content text-xs font-light">
					{m.identity_access_auth_providers_will_own()}
				</span>
			</div>
			<span class="text-success ml-auto flex-none text-xs"
				>{m.identity_access_auth_providers_verified()}</span
			>
		</div>
		<p class="text-muted-content text-xs font-light">
			{m.identity_access_auth_providers_not_right_account()}
			<button class="text-link underline" disabled={switching} onclick={handleVerifyStagedProvider}>
				{m.identity_access_auth_providers_sign_in_again()}
			</button>
		</p>
		<div class="notification-alert flex items-start gap-2 text-sm font-light">
			<TriangleAlert class="mt-0.5 size-5 shrink-0 text-warning" />
			<span>
				{m.identity_access_auth_providers_switch_warning({
					name: `${configuringAuthProvider?.name}`,
					current: activeProvider?.name ?? m.identity_access_auth_providers_current_provider()
				})}
			</span>
		</div>
	{/if}
{/snippet}

{#snippet switchFooter(submit: () => void)}
	{#if switchStep !== 'configure'}
		<IconButton
			variant="danger"
			disabled={switching}
			tooltip={{
				text: m.identity_access_auth_providers_discard_staged_switch(),
				disablePortal: true
			}}
			onclick={() => (confirmDiscardSwitch = true)}
		>
			<Trash2 class="size-5" />
		</IconButton>
	{/if}
	<div class="grow"></div>
	{#if switchStep === 'configure'}
		<button class="btn" disabled={loading} onclick={() => providerConfigure?.close()}>
			{m.common_cancel()}
		</button>
		<button class="btn btn-primary" disabled={loading} onclick={submit}>{m.core_continue()}</button>
	{:else if switchStep === 'signin'}
		{#if !configurationLocked}
			<button class="btn" disabled={switching} onclick={() => goToSwitchStep('configure')}>
				<ArrowLeft class="size-4" />
				{m.identity_access_auth_providers_configuration()}
			</button>
		{/if}
		<button class="btn btn-primary" disabled={switching} onclick={handleVerifyStagedProvider}>
			{m.identity_access_auth_providers_sign_in_with({ name: `${configuringAuthProvider?.name}` })}
		</button>
	{:else}
		<button class="btn" disabled={switching} onclick={() => goToSwitchStep('signin')}>
			<ArrowLeft class="size-4" />
			{m.identity_access_auth_providers_step_signin()}
		</button>
		<button class="btn btn-primary" disabled={switching} onclick={() => (confirmSwitch = true)}>
			{m.identity_access_auth_providers_switch_to({ name: `${configuringAuthProvider?.name}` })}
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
	parameterNotice={scimParameterNotice}
	title={isSwitching
		? m.identity_access_auth_providers_switch_to({ name: `${configuringAuthProvider?.name}` })
		: undefined}
	steps={isSwitching ? switchSteps : undefined}
	body={isSwitching && switchStep !== 'configure' ? switchBody : undefined}
	footer={isSwitching ? switchFooter : undefined}
>
	{#snippet note()}
		{@const documentationUrl = getDocumentationUrl(configuringAuthProvider?.id)}
		{#if residualProvider && residualProvider.id === configuringAuthProvider?.id}
			<div class="notification-alert flex flex-col gap-2 p-3 text-sm font-light" role="alert">
				{#if residualCleanupStarted}
					<p>
						{m.identity_access_auth_providers_scim_cleanup_started({
							name: residualProvider.name
						})}
					</p>
				{:else}
					<p>
						{m.identity_access_auth_providers_scim_still_has_groups({
							name: residualProvider.name
						})}
					</p>
					{#if residualProvider.staged}
						<p>
							{m.identity_access_auth_providers_scim_residual_staged({
								name: residualProvider.name
							})}
						</p>
					{:else}
						<div>
							<button
								class="btn btn-secondary btn-sm"
								type="button"
								disabled={residualCleanupLoading || isReadonly}
								onclick={() => (confirmResidualCleanup = true)}
							>
								{m.identity_access_auth_providers_scim_residual_title()}
							</button>
						</div>
					{/if}
				{/if}
			</div>
		{/if}
		{@const callbackUrl = window.location.protocol + '//' + window.location.host + '/'}
		<div class="notification-info p-3 text-sm font-light">
			<div class="flex items-center gap-3">
				<Info class="size-6" />
				<p class="flex flex-wrap items-center gap-2">
					{m.identity_access_auth_providers_callback_note()}
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
				{m.identity_access_auth_providers_docs_prefix()}<a
					class="text-link"
					href={documentationUrl}
					rel="external noopener noreferrer"
					target="_blank">{m.identity_access_auth_providers_docs_link()}</a
				>{m.identity_access_auth_providers_docs_suffix()}
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
	bootstrap={isBootstrapUser}
	onClose={handleLocalAuthClose}
	switching={atLeastOneConfigured && activeProvider?.id !== CommonAuthProviderIds.LOCAL}
/>

<Confirm
	show={confirmSwitch}
	title={m.identity_access_auth_providers_complete_switch()}
	msg={m.identity_access_auth_providers_switch_to_question({
		name: `${configuringAuthProvider?.name}`
	})}
	submitText={m.identity_access_auth_providers_switch_to({
		name: `${configuringAuthProvider?.name}`
	})}
	note={switchNote}
	cancelText={m.common_cancel()}
	loading={switching}
	onsuccess={handleActivateStagedProvider}
	oncancel={() => (confirmSwitch = false)}
/>

<Confirm
	show={confirmDiscardSwitch}
	title={m.identity_access_auth_providers_discard_switch()}
	msg={m.identity_access_auth_providers_discard_switch_question({
		name: `${configuringAuthProvider?.name}`
	})}
	note={signedInAsVerifiedAccount
		? m.identity_access_auth_providers_discard_note_signed_in({
				name: `${configuringAuthProvider?.name}`,
				email: `${switchVerifiedEmail}`,
				current: activeProvider?.name ?? m.identity_access_auth_providers_current_provider()
			})
		: m.identity_access_auth_providers_discard_note({
				name: `${configuringAuthProvider?.name}`,
				current:
					activeProvider?.name ?? m.identity_access_auth_providers_current_provider_capitalized()
			})}
	submitText={m.identity_access_auth_providers_discard_switch()}
	cancelText={m.identity_access_auth_providers_keep_editing()}
	loading={switching}
	onsuccess={handleUnstageProvider}
	oncancel={() => (confirmDiscardSwitch = false)}
/>

<Confirm
	show={confirmResidualCleanup}
	title={m.identity_access_auth_providers_scim_residual_title()}
	msg={m.identity_access_auth_providers_scim_residual_msg({
		name: residualProvider?.name ?? ''
	})}
	note={residualCleanupNote}
	classes={{ note: 'text-left' }}
	submitText={m.identity_access_auth_providers_scim_residual_submit()}
	cancelText={m.common_cancel()}
	loading={residualCleanupLoading}
	onsuccess={handleRemoveResidualGroupData}
	oncancel={() => (confirmResidualCleanup = false)}
/>

{#snippet residualCleanupNote()}
	<div class="flex flex-col gap-2 text-sm">
		<p>{residualCleanupSummary}</p>
		{#if referencedResidualGroups.length > 0}
			<p>
				{m.identity_access_auth_providers_scim_residual_also({
					name: residualProvider?.name ?? ''
				})}
			</p>
			<ul class="flex max-h-48 list-disc flex-col gap-1 overflow-y-auto pl-5">
				{#each referencedResidualGroups as group (group.id)}
					<li>
						<span class="font-medium">{group.name || group.id}</span>:
						{group.references?.map(describeGroupReference).join('; ')}
					</li>
				{/each}
			</ul>
		{/if}
	</div>
{/snippet}

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
		<h3 class="text-lg font-semibold">
			{m.identity_access_auth_providers_next_step_owner_setup()}
		</h3>
	{/snippet}

	<OwnerSetupPrompt
		{isLocalSetup}
		localUserEmail={setupLocalUserEmail}
		{explicitOwners}
		provider={configuringAuthProvider}
		tempLoginUrl={setupTempLoginUrl}
		onManageLocalUsers={isLocalSetup ? handleManageLocalUsers : undefined}
		showTitle={false}
	/>
</ResponsiveDialog>

<LicenseProviderDialog
	bind:provider={licenseRequiredProvider}
	allowSignup={!licenseRequiredProvider?.configured}
	licenseKey={license.current.licenseKey}
	endpoint={AdminService.createCommunityLicense}
	onSubmit={handleCommunitySubmit}
	signUpMessage={m.identity_access_auth_providers_signup_message()}
/>
