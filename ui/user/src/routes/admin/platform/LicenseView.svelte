<script lang="ts">
	import { page } from '$app/state';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import CommunitySignUpForm from '$lib/components/admin/license/CommunitySignUpForm.svelte';
	import CommunitySignupPanel from '$lib/components/admin/license/CommunitySignupPanel.svelte';
	import UserLimitNotice from '$lib/components/admin/license/UserLimitNotice.svelte';
	import BetaLogo from '$lib/components/navbar/BetaLogo.svelte';
	import { COMMUNITY_SIGNUP_BANNER_COPY, PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { parseErrorContent } from '$lib/errors';
	import { getLocale, m } from '$lib/i18n';
	import { reloadPage } from '$lib/navigation';
	import { AdminService, UserService, type License, type SubscriptionStatus } from '$lib/services';
	import { errors, license as licenseStore, profile, version } from '$lib/stores';
	import { emptyLicense } from '$lib/stores/license.svelte';
	import { setUrlParamAndUpdateUrl } from '$lib/url';
	import { validateVersionUserLimit } from '$lib/utils';
	import {
		CircleAlert,
		CreditCard,
		ExternalLink,
		KeyRound,
		LoaderCircle,
		Lock,
		ReceiptText,
		RefreshCw
	} from '@lucide/svelte';
	import { onMount, untrack } from 'svelte';
	import { fade, slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	let { license: initialLicense }: { license: License | undefined } = $props();

	const SYNC_POLL_INTERVAL = 500;
	const SYNC_POLL_ATTEMPTS = 60;
	const duration = PAGE_TRANSITION_DURATION;

	let license = $state(untrack(() => initialLicense ?? emptyLicense));
	let checking = $state(false);
	let opening = $state(false);
	let showDeleteLicenseDialog = $state(false);
	let deleting = $state(false);
	let now = $state(Date.now());

	let updateLicenseDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let updateLicenseKey = $state('');
	let updating = $state(false);
	let updateError = $state('');

	let billing = $derived(Boolean(version.current.billingEnabled));
	let subscription = $derived(license.subscription);
	let entitlements = $derived(license.entitlements);
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.() ?? false);
	let hasValidLicense = $derived(Boolean(version.current.hasValidLicense));
	let isCommunityEdition = $derived(entitlements.community && !entitlements.enterprise);
	let hasLicenseKey = $derived(Boolean(license.licenseKey) && !isCommunityEdition);
	let canManage = $derived(
		billing &&
			Boolean(subscription?.selfService) &&
			!isAdminReadonly &&
			(profile.current.isOwner?.() ?? false)
	);
	let showEnterpriseCTA = $derived(
		!billing && !isAdminReadonly && (!hasValidLicense || isCommunityEdition)
	);
	let showUserLimitNotice = $derived(
		!billing && !isAdminReadonly && validateVersionUserLimit(version.current)
	);
	let updateLicenseTitle = $derived(
		hasLicenseKey ? m.platform_license_update_key() : m.platform_license_add_key()
	);

	let edition = $derived(
		entitlements.cloud
			? 'Obot Cloud'
			: entitlements.enterprise
				? 'Obot Enterprise'
				: entitlements.community
					? 'Obot Community'
					: m.platform_license_edition_open_source()
	);

	const statusLabels: Record<SubscriptionStatus, () => string> = {
		trialing: m.platform_billing_status_trialing,
		active: m.platform_billing_status_active,
		pastDue: m.platform_billing_status_past_due,
		ended: m.platform_billing_status_ended
	};

	let periodLabel = $derived.by(() => {
		if (!subscription?.periodEnd) return '';
		const date = new Date(subscription.periodEnd).toLocaleDateString(getLocale(), {
			dateStyle: 'medium'
		});
		if (subscription.status === 'trialing' || subscription.cancelAtPeriodEnd) {
			return m.platform_billing_ends_on({ date });
		}
		if (subscription.status === 'ended') return m.platform_license_period_ended({ date });
		return m.platform_billing_renews_on({ date });
	});

	let features = $derived(
		[
			{ name: 'Cloud', granted: entitlements.cloud },
			{ name: 'Enterprise', granted: entitlements.enterprise },
			{ name: 'Community', granted: entitlements.community },
			{
				name: m.platform_license_feature_auth_providers(),
				granted: entitlements.enterpriseAuthProviders
			},
			{
				name: m.platform_license_feature_model_providers(),
				granted: entitlements.enterpriseModelProviders
			}
		].filter((feature) => feature.granted)
	);

	// Users and devices always, and hosted MCP server limits under billing.
	let limits = $derived(
		[
			{
				label: m.platform_billing_users(),
				used: license.usage.users,
				limit: entitlements.users,
				format: String,
				shown: true
			},
			{
				label: m.platform_billing_devices(),
				used: license.usage.devices,
				limit: entitlements.devices,
				format: String,
				shown: true
			},
			{
				label: m.platform_license_row_hosted_servers(),
				used: license.usage.hostedMcpServers,
				limit: entitlements.hostedMcpServers,
				format: String,
				shown: billing
			},
			{
				label: m.platform_license_row_memory(),
				used: license.usage.memoryMiB,
				limit: entitlements.memoryMiB,
				format: formatMemory,
				shown: billing
			},
			{
				label: m.platform_license_row_cpu(),
				used: license.usage.cpuMillicores,
				limit: entitlements.cpuMillicores,
				format: formatCPU,
				shown: billing
			}
		]
			.filter((limit) => limit.shown)
			.map((limit) => ({
				...limit,
				over: limit.limit !== null && limit.used > limit.limit,
				percent:
					limit.limit === null
						? 0
						: limit.limit === 0
							? limit.used > 0
								? 100
								: 0
							: Math.min(100, (limit.used / limit.limit) * 100)
			}))
	);

	let firstOver = $derived(limits.find((limit) => limit.over));
	let restricted = $derived(
		version.current.licenseEntitlementViolations?.some((v) => v.type === 'restricted') ?? false
	);

	// The most important billing issue: a restriction, an exceeded limit, or an unconfirmed, past due or ended
	// subscription.
	let banner = $derived.by(() => {
		if (!billing) return undefined;
		const detail = firstOver
			? {
					label: firstOver.label,
					used: firstOver.format(firstOver.used),
					limit: firstOver.format(firstOver.limit!),
					over: firstOver.format(firstOver.used - firstOver.limit!)
				}
			: undefined;
		if (restricted) {
			return {
				error: true,
				text: detail
					? m.platform_license_banner_restricted(detail)
					: m.platform_license_banner_restricted_unconfirmed()
			};
		}
		if (detail) return { error: false, text: m.platform_license_banner_over(detail) };
		if (!subscription) return { error: false, text: m.platform_license_banner_unconfirmed() };
		if (subscription.status === 'pastDue') {
			return { error: false, text: m.platform_billing_past_due() };
		}
		if (subscription.status === 'ended') return { error: false, text: m.platform_billing_ended() };
		return undefined;
	});

	let manualCheckAvailableAt = $derived(
		license.manualCheckAvailableAt ? new Date(license.manualCheckAvailableAt).getTime() : 0
	);
	let manualCheckCooldownMs = $derived(Math.max(0, manualCheckAvailableAt - now));
	let canCheck = $derived((billing || hasLicenseKey) && !isAdminReadonly);

	$effect(() => {
		if (!manualCheckAvailableAt || manualCheckCooldownMs <= 0) return;

		now = Date.now();
		const interval = window.setInterval(() => {
			now = Date.now();
		}, 1000);

		return () => window.clearInterval(interval);
	});

	onMount(() => {
		// Billing sends owners back here when they're done, so check for changes.
		if (page.url.searchParams.get('billing') === 'returned') {
			setUrlParamAndUpdateUrl(page.url, 'billing', null);
			if (!isAdminReadonly) check();
		}
	});

	function formatMemory(mib: number) {
		return mib >= 1024 && mib % 1024 === 0 ? `${mib / 1024} GiB` : `${mib} MiB`;
	}

	function formatCPU(millicores: number) {
		return `${millicores / 1000} vCPU`;
	}

	function formatCooldown(ms: number) {
		const totalSeconds = Math.ceil(ms / 1000);
		return `${Math.floor(totalSeconds / 60)}:${(totalSeconds % 60).toString().padStart(2, '0')}`;
	}

	// check rechecks the license, or syncs with billing and waits for the sync to finish.
	async function check() {
		checking = true;
		try {
			let updated = await AdminService.recheckLicense({ dontLogErrors: true });
			for (let attempt = 0; updated.syncing && attempt < SYNC_POLL_ATTEMPTS; attempt++) {
				await new Promise((resolve) => setTimeout(resolve, SYNC_POLL_INTERVAL));
				updated = await UserService.getLicense();
			}
			license = updated;
			licenseStore.initialize(updated);
			// The banners read violations from the version.
			version.initialize(await UserService.getVersion());
		} catch (err) {
			errors.append(m.platform_billing_check_failed({ error: parseErrorContent(err).message }));
		} finally {
			checking = false;
		}
	}

	async function manage() {
		opening = true;
		try {
			const session = await AdminService.manageBilling({ dontLogErrors: true });
			window.location.assign(session.url);
		} catch (err) {
			// Billing's conflict messages are meant to be shown as is.
			errors.append(m.platform_billing_manage_failed({ error: parseErrorContent(err).message }));
			opening = false;
		}
	}

	function openUpdateLicenseDialog() {
		if (license.locked) return;
		updateLicenseKey = '';
		updateError = '';
		updateLicenseDialog?.open();
	}

	async function updateLicense() {
		updating = true;
		updateError = '';
		try {
			await AdminService.updateLicense({ licenseKey: updateLicenseKey }, { dontLogErrors: true });
			updateLicenseDialog?.close();
			reloadPage();
		} catch (err) {
			updateError = err instanceof Error ? err.message : m.platform_license_unknown_error();
		} finally {
			updating = false;
		}
	}

	async function deleteLicense() {
		deleting = true;
		try {
			await AdminService.deleteLicense();
			reloadPage();
		} catch (err) {
			errors.append(m.platform_license_delete_failed({ error: `${err}` }));
		} finally {
			deleting = false;
		}
	}
</script>

<!-- Re-enable the button when the browser restores this page after billing. -->
<svelte:window onpageshow={() => (opening = false)} />

<div class="flex h-full w-full flex-col gap-4 @container" in:fade={{ duration }}>
	{#if showUserLimitNotice}
		<UserLimitNotice />
	{/if}

	{#if banner}
		<div
			class={twMerge(
				'flex items-center justify-between gap-4 rounded-box border px-4 py-3 text-sm',
				banner.error
					? 'border-error/30 bg-error/10 text-error'
					: 'border-warning/30 bg-warning/10 text-warning'
			)}
			role="status"
		>
			<p class="flex items-center gap-3">
				{#if banner.error}
					<Lock class="size-4 shrink-0" />
				{:else}
					<CircleAlert class="size-4 shrink-0" />
				{/if}
				{banner.text}
			</p>
			{#if canManage}
				<button class="btn btn-sm shrink-0" onclick={manage} disabled={opening}>
					{m.platform_license_change_plan()}
				</button>
			{/if}
		</div>
	{:else if !billing && license.licenseKey && !hasValidLicense}
		<div class="notification-alert p-3 text-sm font-light">
			<div class="flex items-center gap-3">
				<CircleAlert class="size-6" />
				<div>
					{m.platform_license_invalid_prefix()}<b class="font-semibold"
						>{m.platform_license_invalid_bold()}</b
					>{m.platform_license_invalid_mid()}
					<a href="mailto:info@obot.ai" class="text-link">info@obot.ai</a>
					{m.platform_license_invalid_suffix()}
				</div>
			</div>
		</div>
	{/if}

	{#if showEnterpriseCTA}
		<aside
			class="flex flex-col gap-4 rounded-box bg-primary p-4 text-primary-content shadow-sm sm:flex-row sm:items-center sm:justify-between sm:p-6"
			aria-labelledby="enterprise-cta-heading"
		>
			<div class="flex min-w-0 flex-col gap-1">
				<h2 id="enterprise-cta-heading" class="text-lg font-semibold tracking-tight">
					{m.platform_license_upgrade()}
				</h2>
				<p class="max-w-md text-sm font-light text-primary-content/85">
					{m.platform_license_upgrade_description()}
				</p>
			</div>
			<a
				href="https://obot.ai/contact-us/"
				target="_blank"
				rel="noopener noreferrer"
				class="btn bg-white text-black hover:bg-white/90"
			>
				{m.platform_license_contact_us()}
				<ExternalLink class="size-4" aria-hidden="true" />
				<span class="sr-only">{m.platform_opens_new_tab()}</span>
			</a>
		</aside>
	{/if}

	<section class="paper gap-1.5">
		<div class="flex items-center justify-between gap-4">
			<h4 class="flex items-center gap-2 font-semibold">
				{#if billing}
					<ReceiptText class="size-4" />
					{m.platform_license_subscription_title()}
				{:else}
					<KeyRound class="size-4" />
					{m.platform_license_key_title()}
				{/if}
			</h4>
			{#if billing && subscription}
				<span
					class={twMerge(
						'badge badge-soft badge-sm',
						subscription.status === 'active' || subscription.status === 'trialing'
							? 'badge-success'
							: 'badge-warning'
					)}
				>
					{statusLabels[subscription.status]?.() ?? subscription.status}
				</span>
			{:else if !billing && hasLicenseKey}
				<span
					class={twMerge(
						'badge badge-soft badge-sm',
						hasValidLicense ? 'badge-success' : 'badge-error'
					)}
				>
					{hasValidLicense ? m.platform_license_valid() : m.platform_license_invalid()}
				</span>
			{/if}
		</div>
		{#if billing}
			<p class="text-sm">{subscription?.plan ?? m.platform_license_subscription_waiting()}</p>
			{#if periodLabel}
				<p class="text-sm text-muted-content">{periodLabel}</p>
			{/if}
			{#if subscription && !subscription.selfService}
				<p class="text-sm text-muted-content">{m.platform_billing_managed_by_team()}</p>
			{/if}
		{:else}
			<p class="text-sm">{edition}</p>
			{#if hasLicenseKey}
				<p class="font-mono text-sm text-muted-content">{license.licenseKey}</p>
			{/if}
			{#if license.locked}
				<p class="text-sm text-muted-content">{m.platform_license_managed_by_configuration()}</p>
			{/if}
		{/if}
		{#if !isAdminReadonly && (canManage || !billing)}
			<div class="mt-3 flex flex-wrap gap-2">
				{#if canManage}
					<button
						class="btn btn-secondary btn-sm"
						onclick={manage}
						disabled={opening}
						aria-busy={opening}
					>
						{#if opening}
							<LoaderCircle class="size-4 animate-spin" />
						{:else}
							<CreditCard class="size-4" />
						{/if}
						{m.platform_billing_manage()}
					</button>
				{:else}
					<div
						use:tooltip={{
							text: license.locked ? m.platform_license_locked() : undefined,
							classes: ['text-xs']
						}}
					>
						<button
							class="btn btn-secondary btn-sm"
							onclick={openUpdateLicenseDialog}
							disabled={license.locked}
						>
							{updateLicenseTitle}
						</button>
					</div>
					{#if hasLicenseKey && !license.locked}
						<button
							class="btn btn-ghost btn-sm text-error"
							onclick={() => (showDeleteLicenseDialog = true)}
						>
							{m.platform_license_delete()}
						</button>
					{/if}
				{/if}
			</div>
		{/if}
	</section>

	<section class="paper gap-5">
		<h4 class="font-semibold">{m.platform_license_limits()}</h4>
		{#each limits as limit (limit.label)}
			<div
				class="flex flex-col gap-2"
				aria-label={limit.limit === null
					? m.platform_license_limit_aria_unlimited({
							label: limit.label,
							used: limit.format(limit.used)
						})
					: m.platform_license_limit_aria({
							label: limit.label,
							used: limit.format(limit.used),
							limit: limit.format(limit.limit)
						})}
			>
				<div class="flex items-center justify-between gap-4 text-sm">
					<span>{limit.label}</span>
					<span class={limit.over ? 'text-error' : 'text-muted-content'}>
						{#if limit.limit === null}
							{m.platform_license_value_unlimited({ used: limit.format(limit.used) })}
						{:else if limit.over}
							{m.platform_license_value_over({
								used: limit.format(limit.used),
								limit: limit.format(limit.limit),
								over: limit.format(limit.used - limit.limit)
							})}
						{:else}
							{m.platform_license_value_of({
								used: limit.format(limit.used),
								limit: limit.format(limit.limit)
							})}
						{/if}
					</span>
				</div>
				<div class="h-1.5 w-full overflow-hidden rounded-full bg-base-300">
					<div
						class={twMerge('h-full rounded-full', limit.over ? 'bg-error' : 'bg-primary')}
						style:width={`${limit.percent}%`}
					></div>
				</div>
			</div>
		{/each}
	</section>

	<div class="flex flex-wrap items-center justify-between gap-4">
		<div class="flex flex-wrap items-center gap-x-6 gap-y-2 text-sm">
			{#if features.length > 0}
				<div class="flex flex-wrap items-center gap-2">
					<span class="text-muted-content">{m.platform_license_features()}</span>
					{#each features as feature (feature.name)}
						<span class="badge badge-soft badge-primary badge-sm">{feature.name}</span>
					{/each}
				</div>
			{/if}
			{#if billing}
				<p>
					<span class="text-muted-content">{m.platform_license_retention()}</span>
					<span class="ml-2">
						{entitlements.auditLogRetentionDays === null
							? m.platform_billing_retention_default()
							: m.platform_license_retention_days({ days: entitlements.auditLogRetentionDays })}
					</span>
				</p>
			{/if}
		</div>
		{#if canCheck}
			<button
				class="btn btn-secondary btn-sm"
				onclick={check}
				disabled={checking || manualCheckCooldownMs > 0}
				aria-busy={checking}
			>
				{#if checking}
					<LoaderCircle class="size-4 animate-spin" />
				{:else}
					<RefreshCw class="size-4" />
				{/if}
				{manualCheckCooldownMs > 0
					? m.platform_license_recheck_in({ time: formatCooldown(manualCheckCooldownMs) })
					: m.platform_billing_check()}
			</button>
		{/if}
	</div>

	{#if !billing && !isAdminReadonly && !hasLicenseKey && !isCommunityEdition}
		<CommunitySignupPanel as="aside" class="mx-auto md:max-w-md" labelledBy="community-cta-heading">
			<div class="flex flex-col gap-4 p-4 sm:p-6">
				<BetaLogo class="mx-auto" />
				<p class="max-w-md text-sm font-light">
					{COMMUNITY_SIGNUP_BANNER_COPY}
				</p>
				<div
					class="rounded-xl border border-base-300/80 bg-base-100/80 p-4 shadow-sm backdrop-blur-sm"
				>
					<CommunitySignUpForm
						endpoint={AdminService.createCommunityLicense}
						onSubmit={reloadPage}
						showHeader={false}
						idPrefix="license-community"
						disabled={isAdminReadonly}
					/>
				</div>
			</div>
		</CommunitySignupPanel>
	{/if}
</div>

<ResponsiveDialog bind:this={updateLicenseDialog} title={updateLicenseTitle} class="max-w-md">
	<div class="flex flex-col gap-4">
		<p class="text-sm font-light">{m.platform_license_enter_new_key()}</p>
		<SensitiveInput name="license-key" bind:value={updateLicenseKey} />
		{#if updateError}
			<div in:slide={{ duration: 150, axis: 'y' }} class="alert alert-error alert-soft">
				{updateError}
			</div>
		{/if}
		<button class="btn btn-primary" disabled={updating || isAdminReadonly} onclick={updateLicense}>
			{m.platform_submit()}
		</button>
	</div>
</ResponsiveDialog>

<Confirm
	show={showDeleteLicenseDialog}
	disabled={isAdminReadonly}
	onsuccess={deleteLicense}
	oncancel={() => (showDeleteLicenseDialog = false)}
	msg={m.platform_license_delete_confirm()}
	submitText={m.platform_license_delete()}
	loading={deleting}
/>
