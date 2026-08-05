<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import UserLimitNotice from '$lib/components/admin/license/UserLimitNotice.svelte';
	import {
		COMMUNITY_ENTITLEMENT,
		ENTERPRISE_ENTITLEMENT,
		MODEL_PROVIDERS_ENTITLEMENT
	} from '$lib/constants';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { parseErrorContent } from '$lib/errors';
	import { reloadPage } from '$lib/navigation';
	import { AdminService } from '$lib/services';
	import { errors, license as licenseStore, profile, version } from '$lib/stores';
	import { validateVersionUserLimit } from '$lib/utils';
	import { CircleAlert, ExternalLink, Info, LoaderCircle, RefreshCw } from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { fade, slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	let { data } = $props();
	const editionEntitlements = new Set([COMMUNITY_ENTITLEMENT, ENTERPRISE_ENTITLEMENT]);
	const lockedLicenseMessage = 'The license key is locked and cannot be updated.';

	let license = $state(untrack(() => data.license));

	let showDeleteLicenseDialog = $state(false);
	let deleting = $state(false);
	let rechecking = $state(false);
	let now = $state(Date.now());

	let updateLicenseDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let updateLicenseKey = $state('');
	let updating = $state(false);
	let updateError = $state('');
	let updateLicenseTitle = $derived(license?.licenseKey ? 'Update License Key' : 'Add License Key');
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	let hasValidLicense = $derived(Boolean(license?.enterprise));
	let isCommunityEdition = $derived(
		license?.entitlements?.includes(COMMUNITY_ENTITLEMENT) ?? false
	);
	let visibleEntitlements = $derived(
		[...(license?.entitlements ?? [])]
			.filter((entitlement) => entitlement !== MODEL_PROVIDERS_ENTITLEMENT)
			.sort((a, b) => Number(!editionEntitlements.has(a)) - Number(!editionEntitlements.has(b)))
	);
	let showEnterpriseCTA = $derived(isCommunityEdition);
	let showCommunityEnrollment = $derived(
		Boolean(license && !hasValidLicense && !license.locked && !isAdminReadonly)
	);
	let showUserLimitNotice = $derived(validateVersionUserLimit(version.current));

	let communityName = $state('');
	let communityEmail = $state('');
	let communityCompany = $state('');
	let communitySaving = $state(false);
	let communityError = $state('');
	let manualCheckAvailableAt = $derived(
		license?.manualCheckAvailableAt ? new Date(license.manualCheckAvailableAt).getTime() : 0
	);
	let manualCheckCooldownMs = $derived(Math.max(0, manualCheckAvailableAt - now));
	let manualCheckCooldownLabel = $derived(formatCooldown(manualCheckCooldownMs));

	$effect(() => {
		if (!manualCheckAvailableAt || manualCheckCooldownMs <= 0) return;

		now = Date.now();
		const interval = window.setInterval(() => {
			now = Date.now();
		}, 1000);

		return () => window.clearInterval(interval);
	});

	function handleOpenUpdateLicenseDialog() {
		if (!license || license.locked) return;
		updateLicenseKey = '';
		updateError = '';
		updateLicenseDialog?.open();
	}

	async function handleUpdateLicense() {
		updating = true;
		updateError = '';
		try {
			await AdminService.updateLicense({ licenseKey: updateLicenseKey }, { dontLogErrors: true });
			updateLicenseDialog?.close();
			reloadPage();
		} catch (err) {
			updateError = err instanceof Error ? err.message : 'An unknown error occurred.';
		} finally {
			updating = false;
		}
	}

	async function handleDeleteLicense() {
		deleting = true;
		try {
			await AdminService.deleteLicense();
			reloadPage();
		} catch (err) {
			errors.append(`Failed to delete license: ${err}`);
		} finally {
			deleting = false;
		}
	}

	async function handleRecheckLicense() {
		if (!license?.licenseKey || manualCheckCooldownMs > 0) return;
		rechecking = true;
		try {
			license = await AdminService.recheckLicense({ dontLogErrors: true });
			licenseStore.initialize(license);
		} catch (err) {
			errors.append(`Failed to recheck license: ${err}`);
		} finally {
			rechecking = false;
		}
	}

	async function handleCommunitySubmit(event: SubmitEvent) {
		event.preventDefault();
		if (communitySaving) return;

		communitySaving = true;
		communityError = '';
		try {
			await AdminService.createCommunityLicense(
				{
					name: communityName.trim(),
					email: communityEmail.trim(),
					company: communityCompany.trim() || undefined
				},
				{ dontLogErrors: true }
			);
			reloadPage();
		} catch (err) {
			communityError =
				parseErrorContent(err).message || 'Failed to obtain an Obot Community license.';
		} finally {
			communitySaving = false;
		}
	}

	function formatCooldown(ms: number) {
		if (ms <= 0) return '';

		const totalSeconds = Math.ceil(ms / 1000);
		const minutes = Math.floor(totalSeconds / 60);
		const seconds = totalSeconds % 60;

		return `${minutes}:${seconds.toString().padStart(2, '0')}`;
	}

	function formatEntitlementName(entitlement: string): string {
		if (entitlement === COMMUNITY_ENTITLEMENT) return 'Obot Community';
		if (entitlement === ENTERPRISE_ENTITLEMENT) return 'Obot Enterprise';
		const name = entitlement.replace(/^OBOT_(?:ENTERPRISE_)?/, '');
		if (name === entitlement) return entitlement;

		const words = name.split('_').filter(Boolean);
		if (words.length === 0) return entitlement;

		return words
			.map((word) => (word.length <= 3 ? word : word.charAt(0) + word.slice(1).toLowerCase()))
			.join(' ');
	}

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout title="License">
	<div class="h-full w-full @container" in:fade={{ duration }} out:fade={{ duration }}>
		<div class="flex flex-col gap-4">
			{#if showUserLimitNotice}
				<UserLimitNotice />
			{/if}
			{#if showEnterpriseCTA}
				<aside
					class="relative overflow-hidden rounded-box border border-primary/25 bg-primary text-primary-content shadow-sm"
					aria-labelledby="enterprise-cta-heading"
				>
					<div class="pointer-events-none absolute inset-0" aria-hidden="true">
						<div
							class="absolute inset-0 bg-linear-to-br from-white/15 via-transparent to-blue-950/25"
						></div>
						<div
							class="absolute -top-10 -right-8 size-36 rounded-full border border-white/20"
						></div>
						<div class="absolute -top-4 -right-2 size-24 rounded-full border border-white/15"></div>
						<div class="absolute -right-14 -bottom-16 size-44 rounded-full bg-white/10"></div>
						<div
							class="absolute top-1/2 -left-6 size-20 -translate-y-1/2 rounded-full border border-white/10"
						></div>
						<div
							class="absolute inset-y-0 right-0 w-1/3 bg-linear-to-l from-white/10 to-transparent"
						></div>
					</div>

					<div
						class="relative flex flex-col gap-5 p-4 sm:flex-row sm:items-center sm:justify-between sm:gap-8 sm:p-6"
					>
						<div class="flex min-w-0 items-center gap-4">
							<div
								class="flex size-16 shrink-0 items-center justify-center rounded-xl bg-white/10 backdrop-blur-sm"
							>
								<img
									src="/user/images/obot-icon-white.svg"
									alt=""
									width="48"
									height="48"
									class="size-12"
									decoding="async"
								/>
							</div>
							<div class="flex min-w-0 flex-col gap-1">
								<h2 id="enterprise-cta-heading" class="text-lg font-semibold tracking-tight">
									Upgrade to Obot Enterprise
								</h2>
								<p class="max-w-md text-sm font-light text-primary-content/85">
									Need dedicated support and higher limits for your organization? Talk with our team
									today!
								</p>
							</div>
						</div>

						<a
							href="https://obot.ai/contact-us/"
							target="_blank"
							rel="noopener noreferrer"
							class="btn btn-primary bg-white text-black transition-transform hover:scale-105"
						>
							Contact Us <ExternalLink class="size-4" aria-hidden="true" />
							<span class="sr-only">(opens in a new tab)</span>
						</a>
					</div>
				</aside>
			{/if}
			{#if license && license.licenseKey && !license.enterprise}
				<div class="notification-alert p-3 text-sm font-light">
					<div class="flex items-center gap-3">
						<CircleAlert class="size-6" />
						<div>
							The license key is <b class="font-semibold">invalid</b>. Please contact support at
							<a href="mailto:info@obot.ai" class="text-link">info@obot.ai</a> to renew your license.
						</div>
					</div>
				</div>
			{:else if license && license.locked}
				<div class="notification-info p-3 text-sm font-light">
					<div class="flex items-center gap-3">
						<Info class="size-6" />
						<div>
							The license key was added via configuration and therefore <b class="font-semibold"
								>read-only</b
							>. It cannot be updated from the UI.
						</div>
					</div>
				</div>
			{/if}

			{#if showCommunityEnrollment}
				<form class="paper flex flex-col gap-4" onsubmit={handleCommunitySubmit}>
					<div class="flex flex-col gap-1">
						<h2 class="text-xl font-semibold">Upgrade to Obot Community</h2>
						<p class="text-muted-content text-sm font-light">
							Get permanent, free access to Obot Community and additional authentication providers,
							including Entra, Okta, JumpCloud, and Auth0, with a one-time registration.
						</p>
					</div>

					<div class="grid gap-4 md:grid-cols-2">
						<label class="flex flex-col gap-1 text-sm font-light" for="community-name">
							Name
							<input
								id="community-name"
								class="text-input-filled"
								name="name"
								type="text"
								autocomplete="name"
								bind:value={communityName}
								required
							/>
						</label>

						<label class="flex flex-col gap-1 text-sm font-light" for="community-email">
							Email
							<input
								id="community-email"
								class="text-input-filled"
								name="email"
								type="email"
								pattern="[^\s@]+@[^\s@.]+(?:\.[^\s@.]+)+"
								title="Enter an email address with a valid domain, such as name@example.com."
								autocomplete="email"
								bind:value={communityEmail}
								required
							/>
						</label>

						<label
							class="flex flex-col gap-1 text-sm font-light md:col-span-2"
							for="community-company"
						>
							Company <span class="text-muted-content text-xs">(optional)</span>
							<input
								id="community-company"
								class="text-input-filled"
								name="company"
								type="text"
								autocomplete="organization"
								bind:value={communityCompany}
							/>
						</label>
					</div>

					{#if communityError}
						<div in:slide={{ duration: 150, axis: 'y' }} class="alert alert-error alert-soft">
							{communityError}
						</div>
					{/if}

					<button class="btn btn-primary w-full sm:w-fit" type="submit" disabled={communitySaving}>
						{#if communitySaving}
							<LoaderCircle class="size-4 animate-spin" />
						{/if}
						{communitySaving ? 'Upgrading to Community Edition...' : 'Upgrade to Obot Community'}
					</button>
				</form>
			{/if}
			<section class="paper flex flex-col @2xl:flex-row gap-6 items-start justify-between">
				<div class="flex flex-col gap-6">
					{#if license}
						{#if license.licenseKey}
							<div class="flex flex-col gap-1">
								<div class="text-sm font-light">License Key</div>
								<div class="font-mono text-sm text-muted-content">
									{license.licenseKey}
								</div>
							</div>
						{/if}
						<div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
							<div class="flex flex-col gap-1">
								<p class="text-sm font-light">License Status</p>
								<p
									class={twMerge(
										'text-sm',
										license.licenseKey && 'uppercase font-medium',
										license.licenseKey
											? license.enterprise
												? 'text-success'
												: 'text-error'
											: 'text-muted-content'
									)}
								>
									{#if license.licenseKey}
										{license.enterprise ? 'Active' : 'Invalid'}
									{:else}
										N/A <span class="text-xs font-light">(Open-Source)</span>
									{/if}
								</p>
							</div>
						</div>
						<div class="flex flex-col gap-1">
							<p class="text-sm font-light">License Entitlements</p>
							{#if license.entitlements}
								<ul class="flex flex-wrap gap-2">
									{#each visibleEntitlements as entitlement (entitlement)}
										<li
											class={twMerge(
												'badge badge-soft badge-sm',
												editionEntitlements.has(entitlement) && 'badge-primary'
											)}
										>
											{formatEntitlementName(entitlement)}
										</li>
									{/each}
								</ul>
							{:else}
								-
							{/if}
						</div>
					{/if}
				</div>
				<div class="flex w-full flex-col gap-2 @2xl:w-fit @2xl:flex-row">
					{#if license.licenseKey}
						<button
							class="btn btn-secondary w-full sm:w-fit"
							onclick={handleRecheckLicense}
							disabled={rechecking || manualCheckCooldownMs > 0 || isAdminReadonly}
						>
							{#if rechecking}
								<LoaderCircle class="size-4 animate-spin" />
							{:else}
								<RefreshCw class="size-4" />
							{/if}
							{manualCheckCooldownMs > 0
								? `Recheck in ${manualCheckCooldownLabel}`
								: 'Recheck License'}
						</button>
					{/if}
					<div
						use:tooltip={{
							text: license.locked ? lockedLicenseMessage : undefined,
							classes: ['text-xs']
						}}
						class="w-full sm:w-fit"
					>
						<button
							class="btn btn-secondary w-full sm:w-fit"
							onclick={handleOpenUpdateLicenseDialog}
							disabled={license.locked || isAdminReadonly}
						>
							{updateLicenseTitle}
						</button>
					</div>
				</div>
			</section>

			{#if version.current.userLimit}
				<section class="paper flex-row justify-between py-4">
					<p class="text-sm">User Limits</p>
					{#if !hasValidLicense || isCommunityEdition}
						<p class="text-sm">{version.current.userCount} / {version.current.userLimit}</p>
					{:else}
						<p class="text-sm text-muted-content">-</p>
					{/if}
				</section>
			{/if}

			{#if version.current.deviceLimit}
				<section class="paper flex-row justify-between py-4">
					<p class="text-sm">Device Limits</p>
					{#if !hasValidLicense || isCommunityEdition}
						<p class="text-sm">{version.current.deviceCount} / {version.current.deviceLimit}</p>
					{:else}
						<p class="text-sm text-muted-content">-</p>
					{/if}
				</section>
			{/if}

			{#if license && license.licenseKey}
				<section class="paper gap-0">
					<h4 class="font-semibold text-xl">Danger Zone</h4>
					<p class="text-sm font-light">
						Destructive actions that could cause irreversible changes. Proceed with caution.
					</p>
					<div class="divider my-6"></div>
					<div class="flex items-center flex-col md:flex-row md:justify-between gap-4">
						<div>
							<p class="font-semibold">Delete License</p>
							<p class="text-sm font-light">
								Removing the license will cause loss of access to license-specific features.
							</p>
						</div>
						<div
							use:tooltip={{
								text: license.locked ? lockedLicenseMessage : undefined,
								classes: ['text-xs']
							}}
							class="md:w-fit w-full"
						>
							<button
								class={twMerge('btn btn-error w-full md:w-fit')}
								disabled={license.locked || isAdminReadonly}
								onclick={() => (showDeleteLicenseDialog = true)}
							>
								Delete License
							</button>
						</div>
					</div>
				</section>
			{/if}
		</div>
	</div>
</Layout>

<ResponsiveDialog bind:this={updateLicenseDialog} title={updateLicenseTitle} class="max-w-md">
	<div class="flex flex-col gap-4">
		<p class="text-sm font-light">Enter the new license key below.</p>
		<SensitiveInput name="license-key" bind:value={updateLicenseKey} />
		{#if updateError}
			<div in:slide={{ duration: 150, axis: 'y' }} class="alert alert-error alert-soft">
				{updateError}
			</div>
		{/if}
		<button
			class="btn btn-primary"
			disabled={updating || isAdminReadonly}
			onclick={handleUpdateLicense}
		>
			Submit
		</button>
	</div>
</ResponsiveDialog>

<Confirm
	show={showDeleteLicenseDialog}
	disabled={isAdminReadonly}
	onsuccess={handleDeleteLicense}
	oncancel={() => (showDeleteLicenseDialog = false)}
	msg="Are you sure you want to delete the license?"
	submitText="Delete License"
	loading={deleting}
/>

<svelte:head>
	<title>Obot | License</title>
</svelte:head>
