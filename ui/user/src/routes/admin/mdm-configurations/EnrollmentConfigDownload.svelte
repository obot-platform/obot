<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { parseErrorContent } from '$lib/errors';
	import Loading from '$lib/icons/Loading.svelte';
	import { toHTMLFromMarkdownWithNewTabLinks } from '$lib/markdown';
	import {
		AdminService,
		type MDMAsset,
		type MDMAssetConfiguration,
		type MDMAssetSource,
		type MDMConfiguration
	} from '$lib/services';
	import {
		defaultMDMValues,
		editableMDMValues,
		mdmFieldProblem,
		mdmTargetLabel,
		saveBlob,
		submittedMDMValues
	} from './platforms';
	import { Download, RefreshCw, Save, Trash2, TriangleAlert } from '@lucide/svelte';
	import { onMount, untrack } from 'svelte';
	import { slide } from 'svelte/transition';

	interface Props {
		configuration: MDMConfiguration;
		readOnly?: boolean;
	}

	let { configuration: givenConfiguration, readOnly = false }: Props = $props();

	let configuration = $state(untrack(() => givenConfiguration));
	let editing = $state(false);
	let assetSource = $state<MDMAssetSource>();
	let assetCatalog = $state<MDMAsset[]>([]);
	let assetsLoading = $state(false);
	let assetsLoadError = $state<string>();
	let selectedIndex = $state(-1);
	let values = $state<Record<string, unknown>>({});
	let saving = $state(false);
	let operationError = $state<string>();
	let downloadLoading = $state(false);
	let upgrading = $state(false);
	let clearing = $state(false);
	let confirmClear = $state(false);
	let mutating = $derived(saving || upgrading || clearing);

	$effect(() => {
		configuration = givenConfiguration;
	});

	let latestAsset = $derived(
		assetCatalog.find((asset) => asset.digest === assetSource?.latestDigest)
	);
	let pinnedAsset = $derived(
		configuration.assetDigest
			? assetCatalog.find((asset) => asset.digest === configuration.assetDigest)
			: undefined
	);
	let pinnedTarget = $derived(
		configuration.platform && configuration.os
			? pinnedAsset?.configurations.find(
					(target) => target.platform === configuration.platform && target.os === configuration.os
				)
			: undefined
	);
	let hasTarget = $derived(
		Boolean(configuration.assetDigest && configuration.platform && configuration.os)
	);
	let pinnedTargetLabel = $derived(
		hasTarget
			? pinnedAsset && pinnedTarget
				? mdmTargetLabel(pinnedAsset, pinnedTarget)
				: `${configuration.platform} / ${configuration.os}`
			: ''
	);
	let selected = $derived(latestAsset?.configurations[selectedIndex]);
	let catalogHasTargets = $derived((latestAsset?.configurations.length ?? 0) > 0);
	let catalogError = $derived(assetsLoadError ?? assetSource?.syncError);
	let currentTargetInLatest = $derived(
		Boolean(
			hasTarget &&
			latestAsset?.configurations.some(
				(target) => target.platform === configuration.platform && target.os === configuration.os
			)
		)
	);
	let newerBundleAvailable = $derived(
		Boolean(hasTarget && latestAsset && configuration.assetDigest !== latestAsset.digest)
	);
	let canUpgrade = $derived(newerBundleAvailable && currentTargetInLatest);
	let formFields = $derived(
		Object.entries(latestAsset?.fields.properties ?? {}).filter(
			([, field]) => !field.readOnly && !field.hidden
		)
	);
	let requiredFields = $derived(new Set(latestAsset?.fields.required ?? []));
	let formValid = $derived(
		formFields.every(
			([fieldName, field]) => !mdmFieldProblem(fieldName, field, values[fieldName], requiredFields)
		)
	);

	onMount(() => {
		void loadAssets();
	});

	function targetLabel(configuration: MDMAssetConfiguration): string {
		return latestAsset ? mdmTargetLabel(latestAsset, configuration) : configuration.osLabel;
	}

	async function loadAssets(selectCurrent = false) {
		assetsLoading = true;
		assetsLoadError = undefined;
		if (selectCurrent) {
			selectedIndex = -1;
			values = {};
		}

		try {
			[assetSource, assetCatalog] = await Promise.all([
				AdminService.getMDMAssetSource(),
				AdminService.listMDMAssets()
			]);
			const latest = assetCatalog.find((asset) => asset.digest === assetSource?.latestDigest);
			if (selectCurrent && hasTarget && latest) {
				selectedIndex = latest.configurations.findIndex(
					(target) => target.platform === configuration.platform && target.os === configuration.os
				);
				if (selectedIndex >= 0) {
					values = editableMDMValues(latest.fields, configuration.values ?? {});
				}
			}
		} catch (error) {
			assetsLoadError = parseErrorContent(error).message;
		} finally {
			assetsLoading = false;
		}
	}

	async function startChange() {
		editing = true;
		operationError = undefined;
		await loadAssets(true);
	}

	function cancelChange() {
		editing = false;
		assetsLoadError = undefined;
		selectedIndex = -1;
		values = {};
		operationError = undefined;
	}

	function handleTargetChange(event: Event) {
		selectedIndex = Number((event.currentTarget as HTMLSelectElement).value);
		const target = latestAsset?.configurations[selectedIndex];
		if (!target) {
			values = {};
		} else if (
			hasTarget &&
			target.platform === configuration.platform &&
			target.os === configuration.os
		) {
			values = editableMDMValues(latestAsset?.fields ?? {}, configuration.values ?? {});
		} else {
			values = defaultMDMValues(latestAsset?.fields ?? {});
		}
		operationError = undefined;
	}

	async function handleSave() {
		const asset = latestAsset;
		const target = selected;
		if (!asset || !target || !formValid) return;
		saving = true;
		operationError = undefined;
		try {
			configuration = await AdminService.updateMDMConfiguration({
				...configuration,
				assetDigest: asset.digest,
				platform: target.platform,
				os: target.os,
				values: submittedMDMValues(values)
			});
			cancelChange();
		} catch (error) {
			const problem = parseErrorContent(error);
			operationError = problem.message;
			if (problem.status === 409) {
				await loadAssets(true);
				operationError = latestAsset
					? `${problem.message} The target list was reloaded; review and save again.`
					: `${problem.message} Reload the target list before retrying.`;
			}
		} finally {
			saving = false;
		}
	}

	async function handleClear() {
		clearing = true;
		operationError = undefined;
		try {
			configuration = await AdminService.updateMDMConfiguration({
				...configuration,
				assetDigest: undefined,
				platform: undefined,
				os: undefined,
				values: undefined,
				instructions: undefined,
				error: undefined
			});
			confirmClear = false;
			if (editing) cancelChange();
		} catch (error) {
			operationError = parseErrorContent(error).message;
		} finally {
			clearing = false;
		}
	}

	async function handleDownload() {
		downloadLoading = true;
		operationError = undefined;
		try {
			const { blob, filename } = await AdminService.downloadMDMConfig(configuration.id);
			saveBlob(blob, filename);
		} catch (error) {
			operationError = parseErrorContent(error).message;
		} finally {
			downloadLoading = false;
		}
	}

	async function handleUpgrade() {
		const asset = latestAsset;
		if (!canUpgrade || !asset || !configuration.platform || !configuration.os) return;
		upgrading = true;
		operationError = undefined;
		try {
			configuration = await AdminService.updateMDMConfiguration({
				...configuration,
				assetDigest: asset.digest,
				platform: configuration.platform,
				os: configuration.os,
				values: submittedMDMValues(editableMDMValues(asset.fields, configuration.values ?? {}))
			});
		} catch (error) {
			operationError = parseErrorContent(error).message;
			await loadAssets();
		} finally {
			upgrading = false;
		}
	}
</script>

<div class="paper flex flex-col gap-4 p-4">
	<div class="flex flex-col gap-1">
		<span class="text-sm font-medium">Deployment Target</span>
		<span class="text-muted text-xs">
			The saved platform and OS assets used to generate enrollment packages.
		</span>
	</div>

	{#if configuration.error}
		<div class="notification-alert flex items-start gap-2 p-3 text-sm">
			<TriangleAlert class="size-5 shrink-0 text-warning" />
			<span class="break-all">{configuration.error}</span>
		</div>
	{/if}

	{#if editing}
		<div class="flex flex-col gap-4">
			<div class="flex flex-col gap-1">
				<span class="text-sm font-medium">Choose a target</span>
				<span class="text-muted-content text-xs">
					Saving pins the selected target from the latest stored bundle.
				</span>
			</div>

			{#if assetsLoading}
				<Loading class="size-4 self-center" />
			{:else}
				{#if catalogError}
					<div class="notification-alert flex items-start gap-3 p-3">
						<TriangleAlert class="size-5 shrink-0 text-warning" />
						<div class="flex flex-1 flex-col gap-1">
							<span class="text-sm font-medium">
								{catalogHasTargets
									? 'The latest catalog import failed'
									: 'The deployment catalog is unavailable'}
							</span>
							<span class="text-muted-content text-xs break-all">{catalogError}</span>
						</div>
						{#if assetsLoadError}
							<button
								class="btn btn-secondary btn-sm flex items-center gap-1"
								onclick={() => loadAssets(true)}
							>
								<RefreshCw class="size-3.5" /> Retry
							</button>
						{/if}
					</div>
				{/if}

				{#if hasTarget && latestAsset && catalogHasTargets && !currentTargetInLatest}
					<div class="notification-alert flex items-start gap-3 p-3">
						<TriangleAlert class="size-5 shrink-0 text-warning" />
						<div class="flex flex-col gap-1">
							<span class="text-sm font-medium">The pinned target is not in the latest bundle</span>
							<span class="text-muted-content text-xs">
								The existing download remains usable until you save a replacement target.
							</span>
						</div>
					</div>
				{/if}

				{#if assetSource && !catalogHasTargets && !catalogError}
					<div class="rounded-lg border border-dashed p-4 text-sm text-muted-content">
						No deployment targets are available in the latest bundle.
					</div>
				{/if}

				<div class="flex flex-col gap-2">
					<label for="mdm-change-target" class="input-label">Deployment Target</label>
					<select
						id="mdm-change-target"
						value={selectedIndex}
						onchange={handleTargetChange}
						disabled={!catalogHasTargets}
						class="text-input-filled"
					>
						<option value={-1}>Select a deployment target…</option>
						{#each latestAsset?.configurations ?? [] as configuration, index (`${configuration.platform}/${configuration.os}`)}
							<option value={index}>{targetLabel(configuration)}</option>
						{/each}
					</select>
				</div>

				{#if selected}{@render fieldsForm()}{/if}
			{/if}

			{#if operationError}{@render operationErrorAlert()}{/if}

			<div class="flex justify-end gap-2">
				<button class="btn btn-secondary" disabled={saving} onclick={cancelChange}>Cancel</button>
				<button
					class="btn btn-primary flex items-center gap-2"
					disabled={!selected || !formValid || saving || assetsLoading}
					onclick={handleSave}
				>
					{#if saving}<Loading class="size-4" />{:else}<Save class="size-4" />{/if}
					Save target
				</button>
			</div>
		</div>
	{:else if !hasTarget}
		<div class="flex items-center justify-between gap-4 rounded-lg border border-dashed p-4">
			<div class="flex flex-col gap-1">
				<span class="text-sm font-medium">No deployment target</span>
				<span class="text-muted-content text-xs">
					Devices can enroll, but no MDM package can be generated until a target is chosen.
				</span>
			</div>
			{#if !readOnly}
				<button class="btn btn-primary shrink-0" onclick={startChange}>Choose target</button>
			{/if}
		</div>
	{:else}
		<div class="flex flex-col gap-4">
			<div class="flex items-start justify-between gap-3 rounded-lg border p-4">
				<div class="flex flex-col gap-1">
					<span class="text-sm font-medium">{pinnedTargetLabel}</span>
					{#if pinnedAsset && pinnedTarget}
						<span class="text-muted-content text-xs font-mono">
							{configuration.platform} / {configuration.os}
						</span>
					{/if}
					<span class="text-muted-content text-xs">
						{#if pinnedAsset?.obotSentryVersion}ObotSentry {pinnedAsset.obotSentryVersion} ·
						{/if}<span class="font-mono">{configuration.assetDigest?.slice(0, 12)}</span>
					</span>
				</div>
				<span class="bg-success/10 text-success rounded-full px-2 py-1 text-xs font-medium">
					Pinned
				</span>
			</div>

			<p class="text-muted-content text-xs">
				This configuration keeps this bundle until you explicitly change, upgrade, or clear the
				target.
			</p>

			{#if newerBundleAvailable && !currentTargetInLatest}
				<div class="notification-alert flex items-start gap-2 p-3 text-sm">
					<TriangleAlert class="size-5 shrink-0 text-warning" />
					<span>
						A newer bundle is stored, but it does not contain this platform and OS. The pinned
						deployment is unchanged.
					</span>
				</div>
			{/if}

			{#if operationError}{@render operationErrorAlert()}{/if}

			<div class="flex flex-wrap justify-end gap-2">
				{#if !readOnly}
					{#if canUpgrade}
						<button
							class="btn btn-primary flex items-center gap-2 text-sm"
							disabled={mutating}
							onclick={handleUpgrade}
						>
							{#if upgrading}<Loading class="size-4" />{:else}<RefreshCw class="size-4" />{/if}
							Upgrade{latestAsset?.obotSentryVersion ? ` to ${latestAsset.obotSentryVersion}` : ''}
						</button>
					{/if}
					<button
						class="btn btn-secondary flex items-center gap-2 text-sm"
						disabled={mutating}
						onclick={startChange}
					>
						<RefreshCw class="size-4" /> Change
					</button>
					<button
						class="btn btn-secondary flex items-center gap-2 text-sm text-error"
						disabled={mutating}
						onclick={() => (confirmClear = true)}
					>
						<Trash2 class="size-4" /> Clear
					</button>
				{/if}
				<button
					class="btn btn-primary flex items-center gap-2 text-sm"
					disabled={downloadLoading || mutating || Boolean(configuration.error)}
					onclick={handleDownload}
				>
					{#if downloadLoading}<Loading class="size-4" />{:else}<Download class="size-4" />{/if}
					Download
				</button>
			</div>

			{#if configuration.instructions}
				<div
					class="instructions milkdown-content border-base-300 border-t pt-4"
					transition:slide={{ duration: PAGE_TRANSITION_DURATION }}
				>
					<!-- eslint-disable-next-line svelte/no-at-html-tags -- sanitized by toHTMLFromMarkdownWithNewTabLinks -->
					{@html toHTMLFromMarkdownWithNewTabLinks(configuration.instructions)}
				</div>
			{/if}
		</div>
	{/if}
</div>

{#snippet fieldsForm()}
	<div class="flex flex-col gap-4">
		{#if formFields.length === 0}
			<p class="text-muted-content text-sm">This target has no editable deployment values.</p>
		{/if}
		{#each formFields as [fieldName, field] (fieldName)}
			{@const problem = mdmFieldProblem(fieldName, field, values[fieldName], requiredFields)}
			<div class="flex flex-col gap-1 text-sm">
				<label for={`mdm-field-${fieldName}`} class="text-muted">{field.title ?? fieldName}</label>
				{#if field.enum}
					<select
						id={`mdm-field-${fieldName}`}
						bind:value={values[fieldName]}
						class="text-input-filled w-fit"
					>
						<option value={undefined}>Select…</option>
						{#each field.enum as option (option)}<option value={option}>{option}</option>{/each}
					</select>
				{:else if field.type === 'boolean'}
					<input
						id={`mdm-field-${fieldName}`}
						type="checkbox"
						checked={values[fieldName] === true}
						onchange={(event) => (values[fieldName] = event.currentTarget.checked)}
					/>
				{:else if field.type === 'integer' || field.type === 'number'}
					<input
						id={`mdm-field-${fieldName}`}
						type="number"
						min={field.minimum}
						max={field.maximum}
						bind:value={values[fieldName]}
						class="text-input-filled w-32"
					/>
				{:else}
					<input
						id={`mdm-field-${fieldName}`}
						type="text"
						bind:value={values[fieldName]}
						class="text-input-filled"
					/>
				{/if}
				{#if problem}
					<span class="text-error text-xs">{problem}</span>
				{:else if field.description}
					<span class="text-muted-content text-xs">{field.description}</span>
				{/if}
			</div>
		{/each}
	</div>
{/snippet}

{#snippet operationErrorAlert()}
	<div class="notification-alert flex items-start gap-2 p-3 text-sm">
		<TriangleAlert class="size-5 shrink-0 text-warning" />
		<span class="break-all">{operationError}</span>
	</div>
{/snippet}

<Confirm
	title="Clear deployment target?"
	type="info"
	msg={`Clear the saved deployment target from "${configuration.name}"?`}
	note="Existing devices, enrollment keys, and previously downloaded packages are unaffected. New packages cannot be generated until another target is chosen."
	show={confirmClear}
	loading={clearing}
	submitText="Clear target"
	onsuccess={handleClear}
	oncancel={() => (confirmClear = false)}
/>

<style>
	.instructions :global(pre) {
		margin-bottom: 1rem;
		overflow-x: auto;
		border-radius: 0.5rem;
		background-color: var(--color-base-200);
	}
	.instructions :global(pre code) {
		display: block;
		padding: 0;
		background-color: transparent;
		white-space: pre;
	}
	:global(.dark) .instructions :global(pre) {
		background-color: var(--color-base-300);
	}
</style>
