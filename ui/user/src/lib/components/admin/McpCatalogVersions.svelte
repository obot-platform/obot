<script lang="ts">
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		UserService,
		type CatalogBulkUpgradePlan,
		type CatalogBulkUpgradeResult,
		type CatalogUpgradeApplyRequest,
		type CatalogUpgradePlan,
		type MCPCatalogEntry,
		type MCPCatalogEntryVersion
	} from '$lib/services';
	import { errors } from '$lib/stores';
	import { success } from '$lib/stores/success';
	import Confirm from '../Confirm.svelte';
	import ResponsiveDialog from '../ResponsiveDialog.svelte';
	import CatalogConfigureForm, { type LaunchFormData } from '../mcp/CatalogConfigureForm.svelte';
	import {
		catalogUpgradeConfiguration,
		catalogUpgradeConfigurationComplete,
		catalogUpgradeForm,
		catalogUpgradeNeedsConfiguration,
		getCatalogUpgradeBlockers
	} from '../mcp/catalogUpgrade';
	import { CircleAlert, ExternalLink, GitCompare, Trash2 } from '@lucide/svelte';
	import { onMount } from 'svelte';

	interface Props {
		catalogID: string;
		entry: MCPCatalogEntry;
		onEntryUpdated?: (entry: MCPCatalogEntry) => void;
	}

	let { catalogID, entry, onEntryUpdated }: Props = $props();
	let versions = $state<MCPCatalogEntryVersion[]>([]);
	let loading = $state(true);
	let busy = $state(false);
	let deleteVersion = $state<MCPCatalogEntryVersion>();
	let rollout = $state<CatalogBulkUpgradePlan>();
	let rolloutResult = $state<CatalogBulkUpgradeResult>();
	let rolloutDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let configureDialog = $state<ReturnType<typeof CatalogConfigureForm>>();
	let configuringPlan = $state<CatalogUpgradePlan>();
	let configureForm = $state<LaunchFormData>();
	let applyRequests = $state<Record<string, CatalogUpgradeApplyRequest>>({});
	let origin = $state('');
	let displayedRolloutResults = $derived.by(() => {
		if (!rolloutResult || !rollout) return [];
		const previewedServerIDs = new Set(rollout.plans.map((plan) => plan.serverID));
		return rolloutResult.results.filter((result) => previewedServerIDs.has(result.serverID));
	});

	onMount(() => {
		origin = window.location.origin;
		void loadVersions();
	});

	async function loadVersions() {
		loading = true;
		try {
			versions = await AdminService.listMCPCatalogEntryVersions(catalogID, entry.id);
		} catch (error) {
			errors.append(`Failed to load catalog versions: ${error}`);
		} finally {
			loading = false;
		}
	}

	function versionURL(version: number) {
		return `${origin}/versioned-mcp-connect/${entry.id}/${version}`;
	}

	async function setDefault(version: number) {
		busy = true;
		try {
			const updated = await AdminService.setDefaultMCPCatalogEntryVersion(
				catalogID,
				entry.id,
				version
			);
			entry = { ...updated, isCatalogEntry: true };
			onEntryUpdated?.(entry);
			success.add(`Version ${version} is now the default`);
		} catch (error) {
			errors.append(`Failed to set the default catalog version: ${error}`);
		} finally {
			busy = false;
		}
	}

	async function removeVersion() {
		if (!deleteVersion) return;
		busy = true;
		try {
			await AdminService.deleteMCPCatalogEntryVersion(catalogID, entry.id, deleteVersion.version);
			deleteVersion = undefined;
			await loadVersions();
		} catch (error) {
			errors.append(`Failed to delete catalog version: ${error}`);
		} finally {
			busy = false;
		}
	}

	async function previewRollout() {
		busy = true;
		rolloutResult = undefined;
		applyRequests = {};
		try {
			rollout = await AdminService.previewMCPCatalogEntryRollout(catalogID, entry.id);
			rolloutDialog?.open();
		} catch (error) {
			errors.append(`Failed to preview catalog rollout: ${error}`);
		} finally {
			busy = false;
		}
	}

	function configure(plan: CatalogUpgradePlan) {
		configuringPlan = plan;
		configureForm = catalogUpgradeForm(plan);
		configureDialog?.open();
	}

	function saveConfiguration() {
		if (!configuringPlan) return;
		applyRequests[configuringPlan.serverID] = {
			...applyRequests[configuringPlan.serverID],
			planID: configuringPlan.id,
			configuration: catalogUpgradeConfiguration(configureForm),
			url: configureForm?.url?.trim() || undefined
		};
		applyRequests = { ...applyRequests };
		configureDialog?.close();
	}

	function confirmOAuthReset(plan: CatalogUpgradePlan) {
		applyRequests[plan.serverID] = {
			...applyRequests[plan.serverID],
			planID: plan.id,
			confirmOAuthReauthorization: true
		};
		applyRequests = { ...applyRequests };
	}

	function planReady(plan: CatalogUpgradePlan) {
		const request = applyRequests[plan.serverID];
		return (
			getCatalogUpgradeBlockers(plan).length === 0 &&
			catalogUpgradeConfigurationComplete(plan, request?.configuration ?? {}, request?.url) &&
			(!plan.oauthReauthorizationRequired || request?.confirmOAuthReauthorization === true)
		);
	}

	async function applyRollout() {
		if (!rollout) return;
		busy = true;
		try {
			const readyPlans = rollout.plans.filter(planReady);
			await Promise.all(
				readyPlans
					.filter((plan) => plan.oauthReauthorizationRequired)
					.map((plan) =>
						entry.manifest.serverUserType === 'multiUser'
							? AdminService.clearMCPCatalogServerOAuth(catalogID, plan.serverID)
							: UserService.clearMcpServerOAuth(plan.serverID)
					)
			);
			rolloutResult = await AdminService.applyMCPCatalogEntryRollout(catalogID, entry.id, {
				plans: Object.fromEntries(
					readyPlans.map((plan) => [
						plan.serverID,
						{ ...applyRequests[plan.serverID], planID: plan.id }
					])
				)
			});
		} catch (error) {
			errors.append(`Failed to apply catalog rollout: ${error}`);
		} finally {
			busy = false;
		}
	}
</script>

<div class="flex flex-col gap-4 pb-8">
	<div class="paper flex flex-col gap-3 p-4 sm:flex-row sm:items-center sm:justify-between">
		<div>
			<h3 class="font-medium">Catalog Versions</h3>
			<p class="text-muted-content text-sm">
				Test exact versions and roll out the current default.
			</p>
		</div>
		<button class="btn btn-primary w-full sm:w-auto" onclick={previewRollout} disabled={busy}>
			{#if busy}<Loading class="size-4" />{:else}<GitCompare class="size-4" />{/if}
			Preview Default Rollout
		</button>
	</div>

	{#if loading}
		<div class="flex h-40 items-center justify-center"><Loading class="size-6" /></div>
	{:else if versions.length === 0}
		<div class="paper text-muted-content p-6 text-center text-sm">No versions are available.</div>
	{:else}
		<div class="grid gap-3">
			{#each [...versions].reverse() as version (version.id)}
				<div class="paper flex flex-col gap-3 p-4">
					<div class="flex flex-wrap items-center gap-2">
						<h4 class="font-semibold">Version {version.version}</h4>
						<span class={version.active ? 'pill-success' : 'pill'}
							>{version.active ? 'Active' : 'Inactive'}</span
						>
						{#if version.version === entry.defaultVersion}<span class="pill-primary">Default</span
							>{/if}
						{#if version.version === entry.latestVersion}<span class="pill">Latest</span>{/if}
					</div>
					{#if version.active}
						<div class="flex flex-col gap-2 sm:flex-row">
							<input
								class="input min-w-0 flex-1 font-mono text-xs"
								readonly
								value={versionURL(version.version)}
							/>
							<button
								type="button"
								class="btn btn-secondary"
								onclick={() => window.open(versionURL(version.version), '_blank', 'noopener')}
							>
								<ExternalLink class="size-4" /> Test URL
							</button>
						</div>
					{/if}
					<div class="flex flex-col gap-2 sm:flex-row sm:justify-end">
						{#if version.active && version.version !== entry.defaultVersion}
							<button
								class="btn btn-secondary"
								onclick={() => setDefault(version.version)}
								disabled={busy}
							>
								Set as Default
							</button>
						{/if}
						{#if !version.active && version.version !== entry.defaultVersion}
							<button
								class="btn btn-error"
								onclick={() => (deleteVersion = version)}
								disabled={busy}
							>
								<Trash2 class="size-4" /> Delete
							</button>
						{/if}
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>

<ResponsiveDialog bind:this={rolloutDialog} title="Catalog Rollout Preview" class="md:max-w-3xl">
	<div class="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto p-4 md:p-0">
		{#if rollout}
			<p class="text-muted-content text-sm">
				Version {rollout.targetVersion} affects {rollout.plans.length} server(s) and {rollout.affectedUsers}
				user(s).
			</p>
			{#each rollout.plans as plan (plan.serverID)}
				<div class="border-base-300 flex flex-col gap-2 rounded-md border p-3">
					<div class="flex flex-wrap items-center justify-between gap-2">
						<span class="font-mono text-sm">{plan.serverID}</span>
						<span class={planReady(plan) ? 'pill-success' : 'pill-warning'}>
							{planReady(plan) ? 'Ready' : 'Needs attention'}
						</span>
					</div>
					<p class="text-sm">Version {plan.sourceVersion} to {plan.targetVersion}</p>
					{#each plan.warnings ?? [] as warning, index (`${warning.code}-${index}`)}
						<p class="notification-warning p-2 text-sm">
							<CircleAlert class="size-4 shrink-0" />
							{warning.message}
						</p>
					{/each}
					{#each getCatalogUpgradeBlockers(plan) as blocker, index (`${blocker}-${index}`)}
						<p class="text-error text-sm">{blocker}</p>
					{/each}
					{#if catalogUpgradeNeedsConfiguration(plan)}
						<button class="btn btn-secondary self-start" onclick={() => configure(plan)}>
							{applyRequests[plan.serverID]?.configuration || applyRequests[plan.serverID]?.url
								? 'Edit Configuration'
								: 'Add Configuration'}
						</button>
					{/if}
					{#if plan.oauthReauthorizationRequired}
						<p class="notification-warning p-2 text-sm">
							<CircleAlert class="size-4 shrink-0" /> Existing OAuth authorization will no longer be valid.
							Users must authorize the server again after the update.
						</p>
						<button
							class="btn btn-secondary self-start"
							onclick={() => confirmOAuthReset(plan)}
							disabled={applyRequests[plan.serverID]?.confirmOAuthReauthorization}
						>
							{applyRequests[plan.serverID]?.confirmOAuthReauthorization
								? 'OAuth Reset Confirmed'
								: 'Confirm OAuth Reset'}
						</button>
					{/if}
				</div>
			{/each}

			{#if rolloutResult}
				<div class="paper flex flex-col gap-2 p-3">
					<h4 class="font-medium">Rollout Results</h4>
					{#each displayedRolloutResults as result (result.serverID)}
						<div class="flex flex-col justify-between gap-1 text-sm sm:flex-row">
							<span class="font-mono">{result.serverID}</span>
							<span class={result.applied ? 'text-success' : 'text-error'}>
								{result.applied ? 'Updated' : result.error || 'Not updated'}
							</span>
						</div>
					{/each}
				</div>
			{/if}

			<div class="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
				<button class="btn btn-secondary" onclick={() => rolloutDialog?.close()}>Close</button>
				{#if !rolloutResult}
					<button
						class="btn btn-primary"
						onclick={applyRollout}
						disabled={busy || !rollout.plans.some(planReady)}
					>
						{#if busy}<Loading class="size-4" />{/if} Apply Ready Updates
					</button>
				{/if}
			</div>
		{/if}
	</div>
</ResponsiveDialog>

<CatalogConfigureForm
	bind:this={configureDialog}
	bind:form={configureForm}
	name={configuringPlan?.targetManifest.name}
	icon={configuringPlan?.targetManifest.icon}
	configurationTitle="Required Upgrade Configuration"
	submitText="Save Configuration"
	onSave={saveConfiguration}
/>

<Confirm
	show={!!deleteVersion}
	title="Delete Catalog Version"
	msg={`Delete version ${deleteVersion?.version}?`}
	note="Only inactive, unreferenced versions can be deleted."
	loading={busy}
	onsuccess={removeVersion}
	oncancel={() => (deleteVersion = undefined)}
/>
