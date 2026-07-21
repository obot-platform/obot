<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import DatePicker from '$lib/components/DatePicker.svelte';
	import DotDotDot from '$lib/components/DotDotDot.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Table from '$lib/components/table/Table.svelte';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		type MDMAsset,
		type MDMAssetSource,
		type MDMConfiguration,
		type MDMEnrollmentKey
	} from '$lib/services';
	import { formatTimeAgo, formatTimeUntil } from '$lib/time';
	import EnrollmentConfigDownload from './EnrollmentConfigDownload.svelte';
	import EnrollmentKeyRevealDialog from './EnrollmentKeyRevealDialog.svelte';
	import { KeyRound, Plus, Trash2 } from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { SvelteDate } from 'svelte/reactivity';

	interface Props {
		configuration: MDMConfiguration;
		enrollmentKeys: MDMEnrollmentKey[];
		assetSource?: MDMAssetSource;
		assets: MDMAsset[];
		assetLoadError?: string;
		readOnly?: boolean;
	}

	let {
		configuration,
		enrollmentKeys: initialEnrollmentKeys,
		assetSource,
		assets,
		assetLoadError,
		readOnly = false
	}: Props = $props();

	let enrollmentKeys = $state<MDMEnrollmentKey[]>(untrack(() => initialEnrollmentKeys));
	let revokingKey = $state<MDMEnrollmentKey>();
	let revokeLoading = $state(false);
	let createKeyDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let newKeyName = $state('');
	let newKeyExpiresAt = $state<Date | null>(oneYearFromNow());
	let createKeyLoading = $state(false);
	let revealedCredential = $state<string>();

	function oneYearFromNow(): Date {
		const date = new SvelteDate();
		date.setFullYear(date.getFullYear() + 1);
		return date;
	}

	const keyTableData = $derived(
		enrollmentKeys.map((key) => ({
			...key,
			nameDisplay: key.name || `Key #${key.id}`,
			prefix: `ode1-${configuration.id}-${key.id}-*****`,
			createdAtDisplay: formatTimeAgo(key.createdAt).relativeTime,
			lastUsedAtDisplay: key.lastUsedAt ? formatTimeAgo(key.lastUsedAt).relativeTime : 'Never',
			expiresAtDisplay: key.expiresAt ? formatTimeUntil(key.expiresAt).relativeTime : 'Never'
		}))
	);

	function openCreateKeyDialog() {
		newKeyName = '';
		newKeyExpiresAt = oneYearFromNow();
		createKeyDialog?.open();
	}

	async function handleCreateKey() {
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
		if (!keyToRevoke) return;
		revokeLoading = true;
		try {
			await AdminService.deleteMDMEnrollmentKey(configuration.id, keyToRevoke.id);
			enrollmentKeys = enrollmentKeys.filter((key) => key.id !== keyToRevoke.id);
		} finally {
			revokeLoading = false;
			revokingKey = undefined;
		}
	}
</script>

<div class="flex h-full w-full flex-col gap-6">
	<EnrollmentConfigDownload
		{configuration}
		{assetSource}
		initialAssets={assets}
		initialLoadError={assetLoadError}
		{readOnly}
		enrollmentKeyCount={enrollmentKeys.length}
		onCreateEnrollmentKey={openCreateKeyDialog}
	>
		{#snippet enrollmentKeysSection()}
			<section class="paper gap-4">
				<div class="flex flex-wrap items-start justify-between gap-3">
					<div class="flex flex-col gap-1">
						<h3 class="text-lg font-semibold">Enrollment Keys</h3>
						<p class="text-muted-content text-sm font-light">
							Keys used by Obot Sentry to register a new device with Obot.
						</p>
					</div>
					{#if !readOnly}
						<button
							class="btn btn-secondary btn-sm flex shrink-0 items-center gap-1"
							onclick={openCreateKeyDialog}
						>
							<Plus class="size-4" />
							New Key
						</button>
					{/if}
				</div>

				{#if enrollmentKeys.length === 0}
					<div class="my-4 flex flex-col items-center gap-2 self-center text-center">
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
						{#snippet onRenderColumn(property, key)}
							{#if property === 'createdAt'}
								{key.createdAtDisplay}
							{:else if property === 'lastUsedAt'}
								{key.lastUsedAtDisplay}
							{:else if property === 'expiresAt'}
								{key.expiresAtDisplay}
							{:else if property === 'prefix'}
								<span class="whitespace-nowrap">{key.prefix}</span>
							{:else}
								{key[property as keyof typeof key]}
							{/if}
						{/snippet}
						{#snippet actions(key)}
							{#if !readOnly}
								<DotDotDot>
									<button class="menu-button text-error" onclick={() => (revokingKey = key)}>
										<Trash2 class="size-4" />
										Revoke
									</button>
								</DotDotDot>
							{/if}
						{/snippet}
					</Table>
				{/if}
			</section>
		{/snippet}
	</EnrollmentConfigDownload>
</div>

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
			<label for="mdm-key-expires" class="input-label">Expiration Date</label>
			<DatePicker
				id="mdm-key-expires"
				bind:value={newKeyExpiresAt}
				onChange={(date) => (newKeyExpiresAt = date)}
				placeholder="No expiration"
				minDate={new Date()}
			/>
			<p class="input-description">Defaults to one year from today.</p>
		</div>
	</div>
	<div class="mt-6 flex justify-end gap-2">
		<button class="btn btn-secondary" onclick={() => createKeyDialog?.close()}>Cancel</button>
		<button
			class="btn btn-primary flex items-center gap-2"
			disabled={createKeyLoading}
			onclick={handleCreateKey}
		>
			{#if createKeyLoading}<Loading class="size-4" />{/if}
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
