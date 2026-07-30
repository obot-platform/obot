<script lang="ts">
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

{#if showUserLimitBanner}
	<div class="bg-base-100">
		<div class="bg-warning/10 text-warning px-4 py-2 flex justify-between md:justify-center gap-2">
			<div class="flex items-center gap-4 md:gap-0.5 justify-center">
				<ShieldAlert class="text-warning size-4 shrink-0" />
				<p class="text-xs">
					You're {hasUserLimitViolation ? 'at' : 'almost at'} the user limit.
					{userLimitText} Upgrade to Obot Enterprise!
				</p>
			</div>
			<div class="flex items-center gap-2">
				<button class="btn btn-xs btn-warning" onclick={() => resolveLicenseDialog?.open()}>
					Resolve
				</button>
				{#if !hasUserLimitViolation}
					<button
						class="btn btn-circle btn-ghost btn-xs w-fit h-fit p-0.5"
						onclick={handleDismissUserLimitBanner}
						type="button"
						aria-label="Dismiss user limit banner"
					>
						<X class="size-3" />
					</button>
				{/if}
			</div>
		</div>
	</div>
{:else if !warnUserLimit}
	<div class="bg-base-100">
		<div class="bg-warning/10 text-warning px-4 py-2 flex justify-between md:justify-center gap-2">
			<div class="flex items-center gap-4 md:gap-0.5 justify-center">
				<ShieldAlert class="text-warning size-4 shrink-0" />
				<p class="text-xs">
					{#if profile.current.hasAdminAccess?.()}
						Your license is <b class="font-semibold uppercase"
							>{licenseKey ? 'invalid' : 'missing'}</b
						>. For full functionality, it is recommended to resolve the outstanding issues.
					{:else}
						{MCP_CONNECTION_INVALID_LICENSE_MESSAGE}
					{/if}
				</p>
			</div>
			{#if profile.current.hasAdminAccess?.()}
				<button class="btn btn-xs btn-warning" onclick={() => resolveLicenseDialog?.open()}>
					Resolve
				</button>
			{/if}
		</div>
	</div>
{:else if fallback}
	{@render fallback()}
{/if}

<LicenseResolveDialog bind:this={resolveLicenseDialog} {warnUserLimit} />
