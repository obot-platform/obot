<script lang="ts">
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import { m } from '$lib/i18n';
	import { localState } from '$lib/runes/localState.svelte';
	import { MCP_CONNECTION_INVALID_LICENSE_MESSAGE } from '$lib/services/user/constants';
	import { license, profile, version } from '$lib/stores';
	import LicenseResolveDialog from './LicenseResolveDialog.svelte';
	import { ShieldAlert, X } from '@lucide/svelte';
	import type { Snippet } from 'svelte';

	interface Props {
		warnUserLimit: boolean;
		fallback?: Snippet;
	}

	const { warnUserLimit, fallback }: Props = $props();

	let resolveLicenseDialog = $state<ReturnType<typeof LicenseResolveDialog>>();
	let licenseKey = $derived(license.current.licenseKey);

	const DISMISS_USER_LIMIT_BANNER_KEY = '@obot/dismiss-user-limit-banner';
	const dismissedAt = localState<number | undefined>(DISMISS_USER_LIMIT_BANNER_KEY, undefined, {
		parse: (value) => {
			if (!value) return undefined;
			const parsed = JSON.parse(value) as unknown;
			return typeof parsed === 'number' && !Number.isNaN(parsed) ? parsed : undefined;
		}
	});

	// Violations that come from the billing plan.
	const planLimitViolationTypes = new Set([
		'userLimit',
		'deviceLimit',
		'hostedMcpServerLimit',
		'memoryLimit',
		'cpuLimit',
		'restricted'
	]);
	let restricted = $derived(
		version.current.licenseEntitlementViolations?.some(
			(violation) => violation.type === 'restricted'
		) ?? false
	);
	// The license page shows its own plan banner.
	let onLicensePage = $derived(
		page.url.pathname === resolve('/admin/platform') &&
			(page.url.searchParams.get('view') ?? 'license') === 'license'
	);
	let planViolationsOnly = $derived(
		Boolean(version.current.billingEnabled) &&
			(version.current.licenseEntitlementViolations?.every((violation) =>
				planLimitViolationTypes.has(violation.type)
			) ??
				true)
	);
	let showPlanLimitBanner = $derived(
		Boolean(version.current.billingEnabled) &&
			!onLicensePage &&
			(version.current.licenseEntitlementViolations?.some((violation) =>
				planLimitViolationTypes.has(violation.type)
			) ??
				false)
	);

	let hasUserLimitViolation = $derived(
		version.current.licenseEntitlementViolations?.some(
			(violation) => violation.type === 'userLimit'
		)
	);
	let userLimitText = $derived(
		version.current.userLimit && version.current.userCount
			? `(${version.current.userCount}/${version.current.userLimit})`
			: ''
	);

	let showUserLimitBanner = $derived.by(() => {
		if (!profile.current.hasAdminAccess?.()) return false;
		if (!warnUserLimit && !hasUserLimitViolation) return false;

		if (hasUserLimitViolation) return true;
		if (!dismissedAt.isReady) return false;

		const saved = dismissedAt.current;
		if (typeof saved !== 'number') return true;

		const profileCreatedMs = profile.current.created
			? new Date(profile.current.created).getTime()
			: undefined;
		if (
			profileCreatedMs === undefined ||
			Number.isNaN(profileCreatedMs) ||
			profileCreatedMs < saved
		) {
			return false;
		}

		return true;
	});

	function handleDismissUserLimitBanner() {
		dismissedAt.current = Date.now();
	}
</script>

{#if showPlanLimitBanner}
	<div class="bg-base-100">
		<div class="bg-warning/10 text-warning px-4 py-2 flex justify-between md:justify-center gap-2">
			<div class="flex items-center gap-4 md:gap-0.5 justify-center">
				<ShieldAlert class="text-warning size-4 shrink-0" />
				<p class="text-xs">
					{restricted
						? m.platform_license_notice_restricted()
						: m.platform_license_notice_over_plan_limits()}
				</p>
			</div>
			{#if profile.current.hasAdminAccess?.()}
				<a class="btn btn-xs btn-warning" href={resolve('/admin/platform?view=license')}>
					{m.platform_license_notice_view_plan()}
				</a>
			{/if}
		</div>
	</div>
{:else if showUserLimitBanner}
	<div class="bg-base-100">
		<div class="bg-warning/10 text-warning px-4 py-2 flex justify-between md:justify-center gap-2">
			<div class="flex items-center gap-4 md:gap-0.5 justify-center">
				<ShieldAlert class="text-warning size-4 shrink-0" />
				<p class="text-xs">
					{hasUserLimitViolation
						? m.platform_license_notice_banner_at_limit({ limit: userLimitText })
						: m.platform_license_notice_banner_almost_at_limit({ limit: userLimitText })}
				</p>
			</div>
			<div class="flex items-center gap-2">
				<button class="btn btn-xs btn-warning" onclick={() => resolveLicenseDialog?.open()}>
					{m.platform_license_notice_resolve()}
				</button>
				{#if !hasUserLimitViolation}
					<button
						class="btn btn-circle btn-ghost btn-xs w-fit h-fit p-0.5"
						onclick={handleDismissUserLimitBanner}
						type="button"
						aria-label={m.platform_license_notice_dismiss_user_limit_banner()}
					>
						<X class="size-3" />
					</button>
				{/if}
			</div>
		</div>
	</div>
{:else if !warnUserLimit && !planViolationsOnly}
	<div class="bg-base-100">
		<div class="bg-warning/10 text-warning px-4 py-2 flex justify-between md:justify-center gap-2">
			<div class="flex items-center gap-4 md:gap-0.5 justify-center">
				<ShieldAlert class="text-warning size-4 shrink-0" />
				<p class="text-xs">
					{#if profile.current.hasAdminAccess?.() && version.current.billingEnabled}
						{m.platform_license_notice_plan_missing_feature()}
					{:else if profile.current.hasAdminAccess?.()}
						{m.platform_license_notice_status_prefix()}<b class="font-semibold uppercase"
							>{licenseKey
								? m.platform_license_notice_status_invalid()
								: m.platform_license_notice_status_missing()}</b
						>{m.platform_license_notice_status_suffix()}
					{:else}
						{MCP_CONNECTION_INVALID_LICENSE_MESSAGE}
					{/if}
				</p>
			</div>
			{#if profile.current.hasAdminAccess?.()}
				<button class="btn btn-xs btn-warning" onclick={() => resolveLicenseDialog?.open()}>
					{m.platform_license_notice_resolve()}
				</button>
			{/if}
		</div>
	</div>
{:else if fallback}
	{@render fallback()}
{/if}

<LicenseResolveDialog bind:this={resolveLicenseDialog} {warnUserLimit} />
