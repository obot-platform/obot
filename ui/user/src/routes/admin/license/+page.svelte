<script lang="ts">
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import { AdminService } from '$lib/services';
	import { errors, license as licenseStore, profile } from '$lib/stores';
	import { CircleAlert, Info, LoaderCircle, RefreshCw } from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { fade, slide } from 'svelte/transition';
	import { twMerge } from 'tailwind-merge';

	let { data } = $props();

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
			window.location.reload();
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
			window.location.reload();
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

	function formatCooldown(ms: number) {
		if (ms <= 0) return '';

		const totalSeconds = Math.ceil(ms / 1000);
		const minutes = Math.floor(totalSeconds / 60);
		const seconds = totalSeconds % 60;

		return `${minutes}:${seconds.toString().padStart(2, '0')}`;
	}

	function convertUserFriendlyEntitlements(entitlements: string[]): string[] {
		return entitlements.map((entitlement) => {
			switch (entitlement) {
				case 'OBOT_ENTERPRISE_AUTH_PROVIDERS':
					return 'Auth Providers';
				case 'OBOT_ENTERPRISE_MODEL_PROVIDERS':
					return 'Model Providers';
				default:
					return entitlement;
			}
		});
	}

	const duration = PAGE_TRANSITION_DURATION;
</script>

<Layout title="License">
	<div class="h-full w-full" in:fade={{ duration }} out:fade={{ duration }}>
		<div class="flex flex-col gap-4">
			{#if license && !license.licenseKey}
				<div class="notification-info p-3 text-sm font-light">
					<div class="flex items-center gap-3">
						<Info class="size-6" />
						<div>
							Interested in purchasing a license or want to learn more? Contact support at <a
								href="mailto:info@obot.ai"
								class="text-link">info@obot.ai</a
							>.
						</div>
					</div>
				</div>
			{:else if license && license.licenseKey && !license.enterprise}
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
			<div class="paper flex flex-col gap-6">
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
						<div class="flex w-full flex-col gap-2 sm:w-fit sm:flex-row">
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
					</div>
					<div class="flex flex-col gap-1">
						<p class="text-sm font-light">License Entitlements</p>
						{#if license.entitlements}
							<ul class="flex flex-wrap gap-2">
								{#each convertUserFriendlyEntitlements(license.entitlements ?? []) as entitlement (entitlement)}
									<li class="badge badge-soft badge-sm">{entitlement}</li>
								{/each}
							</ul>
						{:else}
							-
						{/if}
					</div>
				{/if}
			</div>

			{#if license && license.licenseKey}
				<div class="paper gap-0">
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
				</div>
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
