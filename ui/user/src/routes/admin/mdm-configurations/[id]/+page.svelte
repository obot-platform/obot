<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import DatePicker from '$lib/components/DatePicker.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService, type MDMEnrollmentKey } from '$lib/services';
	import { profile } from '$lib/stores';
	import { formatTimeAgo, formatTimeUntil } from '$lib/time';
	import EnrollmentConfigDownload from '../EnrollmentConfigDownload.svelte';
	import EnrollmentKeyRevealDialog from '../EnrollmentKeyRevealDialog.svelte';
	import { KeyRound, Laptop, Plus, Trash2 } from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	let { data } = $props();
	const { configuration, devices } = $derived(data);
	let enrollmentKeys = $state<MDMEnrollmentKey[]>(untrack(() => data.enrollmentKeys));

	let title = $derived(configuration?.name ?? 'MDM Configuration');
	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	const duration = PAGE_TRANSITION_DURATION;

	let revokingKey = $state<MDMEnrollmentKey>();
	let revokeLoading = $state(false);

	let createKeyDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let newKeyName = $state('');
	let newKeyExpiresAt = $state<Date | null>(null);
	let createKeyLoading = $state(false);
	let revealedCredential = $state<string>();

	const keyTableData = $derived(
		enrollmentKeys.map((key) => ({
			...key,
			nameDisplay: key.name || `Key #${key.id}`,
			prefix: configuration ? `ode1-${configuration.id}-${key.id}-*****` : `ode1-*-${key.id}-*****`,
			createdAtDisplay: formatTimeAgo(key.createdAt).relativeTime,
			lastUsedAtDisplay: key.lastUsedAt ? formatTimeAgo(key.lastUsedAt).relativeTime : 'Never',
			expiresAtDisplay: key.expiresAt ? formatTimeUntil(key.expiresAt).relativeTime : 'Never'
		}))
	);

	const deviceTableData = $derived(
		(devices ?? []).map((device) => ({
			...device,
			hostnameDisplay: device.hostname || '-',
			osDisplay: [device.os, device.osVersion].filter(Boolean).join(' ') || '-',
			enrolledAtDisplay: formatTimeAgo(device.enrolledAt).relativeTime,
			lastSeenAtDisplay: device.lastSeenAt ? formatTimeAgo(device.lastSeenAt).relativeTime : '-'
		}))
	);

	function openCreateKeyDialog() {
		newKeyName = '';
		newKeyExpiresAt = null;
		createKeyDialog?.open();
	}

	async function handleCreateKey() {
		if (!configuration) return;
		createKeyLoading = true;
		try {
			const response = await AdminService.createMDMEnrollmentKey(configuration.id, {
				name: newKeyName.trim() || undefined,
				expiresAt: newKeyExpiresAt?.toISOString()
			});
			enrollmentKeys = [response, ...enrollmentKeys];
			createKeyDialog?.close();
			revealedCredential = response.enrollmentCredential;
		} finally {
			createKeyLoading = false;
		}
	}

	async function handleRevokeKey() {
		const keyToRevoke = revokingKey;
		if (!configuration || !keyToRevoke) return;
		revokeLoading = true;
		try {
			await AdminService.deleteMDMEnrollmentKey(configuration.id, keyToRevoke.id);
			enrollmentKeys = enrollmentKeys.filter((k) => k.id !== keyToRevoke.id);
		} finally {
			revokeLoading = false;
			revokingKey = undefined;
		}
	}
</script>

<Layout {title} showBackButton>
	<div
		class="flex h-full w-full flex-col gap-6"
		in:fly={{ x: 100, duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if configuration}
			<div class="paper flex flex-col gap-1 p-4">
				<div class="flex items-center justify-between">
					<span class="text-lg font-semibold">{configuration.name}</span>
					<span class="text-muted text-xs">
						Created {formatTimeAgo(configuration.createdAt).relativeTime}
					</span>
				</div>
				{#if configuration.description}
					<p class="text-muted text-sm">{configuration.description}</p>
				{/if}
			</div>

			<EnrollmentConfigDownload {configuration} readOnly={isAdminReadonly} />

			<div class="flex flex-col gap-2">
				<div class="flex items-center justify-between">
					<h4 class="text-base font-semibold">Enrollment Keys</h4>
					{#if !isAdminReadonly}
						<button
							class="btn btn-secondary flex items-center gap-2 text-sm"
							onclick={openCreateKeyDialog}
						>
							<Plus class="size-4" />
							New Key
						</button>
					{/if}
				</div>
				<p class="text-muted text-sm">
					Keys authorize enrolling new devices into this configuration. Revoking a key stops new
					enrollments only — devices already enrolled keep reporting. To rotate: create a new key,
					distribute it through your MDM, then revoke the old one.
				</p>
				{#if enrollmentKeys.length === 0}
					<div class="mt-8 mb-4 flex flex-col items-center gap-2 self-center text-center">
						<KeyRound class="text-muted-content size-12 opacity-50" />
						<p class="text-muted-content text-sm font-light">
							No enrollment keys. New devices cannot enroll until you create one.
						</p>
					</div>
				{:else}
					<Table
						data={keyTableData}
						fields={['nameDisplay', 'prefix', 'createdAt', 'lastUsedAt', 'expiresAt']}
						headers={[
							{ title: 'Name', property: 'nameDisplay' },
							{ title: 'Key', property: 'prefix' },
							{ title: 'Created', property: 'createdAt' },
							{ title: 'Last Used', property: 'lastUsedAt' },
							{ title: 'Expires', property: 'expiresAt' }
						]}
					>
						{#snippet onRenderColumn(property, d)}
							{#if property === 'createdAt'}
								{d.createdAtDisplay}
							{:else if property === 'lastUsedAt'}
								{d.lastUsedAtDisplay}
							{:else if property === 'expiresAt'}
								{d.expiresAtDisplay}
							{:else if property === 'prefix'}
								<span class="whitespace-nowrap">{d.prefix}</span>
							{:else}
								{d[property as keyof typeof d]}
							{/if}
						{/snippet}
						{#snippet actions(d)}
							{#if !isAdminReadonly}
								<DotDotDot>
									<button class="menu-button text-error" onclick={() => (revokingKey = d)}>
										<Trash2 class="size-4" />
										Revoke
									</button>
								</DotDotDot>
							{/if}
						{/snippet}
					</Table>
				{/if}
			</div>

			<div class="flex flex-col gap-2">
				<h4 class="text-base font-semibold">Enrolled Devices</h4>
				{#if deviceTableData.length === 0}
					<div class="mt-8 mb-4 flex flex-col items-center gap-2 self-center text-center">
						<Laptop class="text-muted-content size-12 opacity-50" />
						<p class="text-muted-content text-sm font-light">
							No devices enrolled yet. Devices appear here as soon as they enroll through your MDM
							configuration.
						</p>
					</div>
				{:else}
					<Table
						data={deviceTableData}
						fields={['deviceID', 'hostnameDisplay', 'osDisplay', 'enrolledAt', 'lastSeenAt']}
						headers={[
							{ title: 'Device ID', property: 'deviceID' },
							{ title: 'Hostname', property: 'hostnameDisplay' },
							{ title: 'OS', property: 'osDisplay' },
							{ title: 'Enrolled', property: 'enrolledAt' },
							{ title: 'Last Seen', property: 'lastSeenAt' }
						]}
					>
						{#snippet onRenderColumn(property, d)}
							{#if property === 'deviceID'}
								<span class="font-mono text-xs">{d.deviceID}</span>
							{:else if property === 'enrolledAt'}
								{d.enrolledAtDisplay}
							{:else if property === 'lastSeenAt'}
								{d.lastSeenAtDisplay}
							{:else}
								{d[property as keyof typeof d]}
							{/if}
						{/snippet}
					</Table>
				{/if}
			</div>
		{/if}
	</div>
</Layout>

<ResponsiveDialog bind:this={createKeyDialog} title="New Enrollment Key" class="w-full max-w-md">
	<div class="flex flex-col gap-4">
		<div class="flex flex-col gap-2">
			<label for="mdm-key-name" class="input-label">Name (Optional)</label>
			<input
				id="mdm-key-name"
				type="text"
				bind:value={newKeyName}
				placeholder="e.g. rotation-2026-07"
				class="text-input-filled"
			/>
		</div>
		<div class="flex flex-col gap-2">
			<label for="mdm-key-expires" class="input-label">Expiration Date (Optional)</label>
			<DatePicker
				id="mdm-key-expires"
				bind:value={newKeyExpiresAt}
				onChange={(date) => (newKeyExpiresAt = date)}
				placeholder="No expiration"
				minDate={new Date()}
			/>
			<p class="input-description">Leave empty for no expiration</p>
		</div>
	</div>
	<div class="mt-6 flex justify-end gap-2">
		<button class="btn btn-secondary" onclick={() => createKeyDialog?.close()}>Cancel</button>
		<button
			class="btn btn-primary flex items-center gap-2"
			disabled={createKeyLoading}
			onclick={handleCreateKey}
		>
			{#if createKeyLoading}
				<Loading class="size-4" />
			{/if}
			Create Key
		</button>
	</div>
</ResponsiveDialog>

<EnrollmentKeyRevealDialog
	credential={revealedCredential}
	onClose={() => (revealedCredential = undefined)}
/>

<Confirm
	msg={`Revoke enrollment key "${revokingKey?.name || `#${revokingKey?.id}`}"? New devices can no longer enroll with it; already-enrolled devices are unaffected.`}
	show={Boolean(revokingKey)}
	loading={revokeLoading}
	onsuccess={handleRevokeKey}
	oncancel={() => (revokingKey = undefined)}
/>

<svelte:head>
	<title>Obot | {title}</title>
</svelte:head>
