<script lang="ts">
	import { codeSnippetCopy } from '$lib/actions/codeSnippetCopy';
	import { tooltip } from '$lib/actions/tooltip.svelte';
	import IconButton from '$lib/components/primitives/IconButton.svelte';
	import { saveBlob } from '$lib/download';
	import { parseErrorContent } from '$lib/errors';
	import Loading from '$lib/icons/Loading.svelte';
	import { toHTMLFromMarkdownWithNewTabLinks } from '$lib/markdown';
	import {
		AdminService,
		type MDMAsset,
		type MDMAssetSource,
		type MDMConfiguration
	} from '$lib/services';
	import { formatTimeAgo } from '$lib/time';
	import { editableMDMValues, mdmFieldProblem, submittedMDMValues } from './platforms';
	import { CircleCheck, Download, RefreshCw, Save, Settings, TriangleAlert } from '@lucide/svelte';
	import { onDestroy, onMount, untrack, type Snippet } from 'svelte';

	const rememberedTargetKey = 'obot-device-configuration-target';

	interface TargetOption {
		description?: string;
		os: string;
		osLabel: string;
		platform: string;
		platformLabel: string;
	}

	interface Props {
		configuration: MDMConfiguration;
		assetSource?: MDMAssetSource;
		initialAssets?: MDMAsset[];
		initialLoadError?: string;
		readOnly?: boolean;
		enrollmentKeyCount?: number;
		onCreateEnrollmentKey?: () => void;
		enrollmentKeysSection?: Snippet;
	}

	let {
		configuration: givenConfiguration,
		assetSource: initialAssetSource,
		initialAssets = [],
		initialLoadError,
		readOnly = false,
		enrollmentKeyCount = 0,
		onCreateEnrollmentKey,
		enrollmentKeysSection
	}: Props = $props();

	let configuration = $state(untrack(() => givenConfiguration));
	let assetSource = $state<MDMAssetSource | undefined>(untrack(() => initialAssetSource));
	let assetCatalog = $state<MDMAsset[]>(untrack(() => initialAssets));
	let assetsLoading = $state(false);
	let assetsLoadError = $state<string | undefined>(untrack(() => initialLoadError));
	let selectedIndex = $state(-1);
	let values = $state<Record<string, unknown>>({});
	let saving = $state(false);
	let operationError = $state<string>();
	let downloadLoading = $state(false);
	let checkingForUpdates = $state(false);
	let checkNote = $state<string>();
	let settingsOpen = $state(false);
	let settingsInitialized = false;
	let destroyed = false;

	$effect(() => {
		configuration = givenConfiguration;
	});

	let latestAsset = $derived(
		assetCatalog.find((asset) => asset.digest === assetSource?.latestDigest)
	);
	let latestVersion = $derived(latestAsset?.obotSentryVersion);
	// The backend copies the release version onto the configuration when the
	// downloads are built; blank configurations fall back to the latest release.
	let versionChip = $derived.by(() => {
		if (configuration.obotSentryVersion) return `v${configuration.obotSentryVersion}`;
		if (configuration.assetDigest) return configuration.assetDigest.slice(0, 12);
		return latestVersion ? `v${latestVersion}` : undefined;
	});
	let targetOptions = $derived.by((): TargetOption[] => {
		if (latestAsset?.configurations.length) {
			const asset = latestAsset;
			return asset.configurations.map((target) => ({
				description: target.description,
				os: target.os,
				osLabel: target.osLabel || target.os,
				platform: target.platform,
				platformLabel:
					asset.platforms.find((platform) => platform.id === target.platform)?.label ??
					target.platform
			}));
		}
		return (configuration.artifacts ?? []).map((artifact) => ({
			os: artifact.os,
			osLabel: artifact.os,
			platform: artifact.platform,
			platformLabel: artifact.platform
		}));
	});
	// Targets group by installation method with its operating systems
	// underneath. The manual installation method always sorts first.
	let platformGroups = $derived.by(() => {
		const groups: {
			platform: string;
			platformLabel: string;
			targets: { option: TargetOption; index: number }[];
		}[] = [];
		for (const [index, option] of targetOptions.entries()) {
			let group = groups.find((candidate) => candidate.platform === option.platform);
			if (!group) {
				group = { platform: option.platform, platformLabel: option.platformLabel, targets: [] };
				groups.push(group);
			}
			group.targets.push({ option, index });
		}
		const manualIndex = groups.findIndex((group) => group.platform.toLowerCase() === 'manual');
		if (manualIndex > 0) {
			groups.unshift(...groups.splice(manualIndex, 1));
		}
		return groups;
	});
	let selectedTarget = $derived(targetOptions[selectedIndex]);
	let selectedGroup = $derived(
		platformGroups.find((group) => group.platform === selectedTarget?.platform)
	);
	let selectedArtifact = $derived(selectedTarget ? artifactFor(selectedTarget) : undefined);
	let hasSavedValues = $derived(Boolean(configuration.assetDigest));
	let artifactCount = $derived((configuration.artifacts ?? []).length);
	let catalogHasTargets = $derived((latestAsset?.configurations.length ?? 0) > 0);
	let catalogError = $derived(assetsLoadError ?? assetSource?.syncError);
	let updateAvailable = $derived(
		Boolean(
			configuration.assetDigest && latestAsset && configuration.assetDigest !== latestAsset.digest
		)
	);
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
	let dirty = $derived.by(() => {
		if (!latestAsset) return false;
		// Compare against the form's initial state, not the raw stored values.
		// Stored values are sparse — schema defaults are never persisted — while
		// the form is seeded with those defaults. Running the stored values
		// through the same transform keeps a default-only configuration (e.g. a
		// freshly auto-configured one) from reading as dirty and demanding a save.
		const baseline = editableMDMValues(latestAsset.fields, configuration.values ?? {});
		return (
			configuration.assetDigest !== latestAsset.digest ||
			canonicalValues(baseline) !== canonicalValues(values)
		);
	});
	// One place reports the whole configuration's state; downloads share it, so
	// there is nothing meaningful to badge per target.
	let configState = $derived.by(() => {
		if (updateAvailable) return 'update';
		if (!hasSavedValues) return 'unconfigured';
		if (dirty) return 'unsaved';
		if (artifactCount > 0) return 'current';
		return undefined;
	});
	let downloadNote = $derived.by(() => {
		if (!selectedTarget) return;
		if (selectedArtifact && dirty) return 'Save your changes before downloading.';
		if (!selectedArtifact) return 'Save to generate this download.';
	});

	onMount(() => {
		if (assetSource && assetCatalog.length > 0) {
			initializeFromConfiguration();
			initializeSettingsVisibility();
		} else {
			void loadAssets();
		}
	});

	onDestroy(() => {
		destroyed = true;
	});

	function canonicalValues(source: Record<string, unknown>): string {
		return JSON.stringify(
			Object.fromEntries(
				Object.entries(submittedMDMValues(source)).sort(([left], [right]) =>
					left.localeCompare(right)
				)
			)
		);
	}

	function targetKey(target: Pick<TargetOption, 'platform' | 'os'>): string {
		return `${target.platform}/${target.os}`;
	}

	function artifactFor(target: Pick<TargetOption, 'platform' | 'os'>) {
		return (configuration.artifacts ?? []).find(
			(artifact) => artifact.platform === target.platform && artifact.os === target.os
		);
	}

	function initializeFromConfiguration() {
		const latest = latestAsset;
		values = latest ? editableMDMValues(latest.fields, configuration.values ?? {}) : {};

		let index = -1;
		if (typeof localStorage !== 'undefined') {
			const remembered = localStorage.getItem(rememberedTargetKey);
			index = targetOptions.findIndex((target) => targetKey(target) === remembered);
		}
		if (index < 0) {
			// The manual installation method is the default whenever it exists.
			index = targetOptions.findIndex((target) => target.platform.toLowerCase() === 'manual');
		}
		if (index < 0) {
			index = targetOptions.findIndex((target) => Boolean(artifactFor(target)));
		}
		selectedIndex = index >= 0 ? index : targetOptions.length > 0 ? 0 : -1;
	}

	// The agent settings start open only when they need attention; a healthy
	// configured page goes straight to the steps.
	function initializeSettingsVisibility() {
		if (settingsInitialized) return;
		settingsInitialized = true;
		settingsOpen = configState !== 'current';
	}

	async function loadAssets() {
		assetsLoading = true;
		assetsLoadError = undefined;
		try {
			[assetSource, assetCatalog] = await Promise.all([
				AdminService.getMDMAssetSource(),
				AdminService.listMDMAssets()
			]);
			initializeFromConfiguration();
		} catch (error) {
			assetsLoadError = parseErrorContent(error).message;
			initializeFromConfiguration();
		} finally {
			assetsLoading = false;
			initializeSettingsVisibility();
		}
	}

	// checkForUpdates asks the backend to fetch the latest Obot Sentry release,
	// waits for it to settle, and reloads. When a newer release landed, the
	// backend re-renders this configuration automatically whenever its saved
	// settings still validate; the settings only open for review when that
	// wasn't possible. An uneventful check needs no message of its own: the
	// refreshed "checked just now" time already says it.
	async function checkForUpdates() {
		checkingForUpdates = true;
		checkNote = undefined;
		operationError = undefined;
		try {
			const previousDigest = assetSource?.latestDigest;
			const hadCatalog = Boolean(latestAsset);
			await AdminService.refreshMDMAssetSource();
			let source = await AdminService.getMDMAssetSource();
			const deadline = Date.now() + 120_000;
			while (!destroyed && source.isSyncing && Date.now() < deadline) {
				await new Promise((resolve) => setTimeout(resolve, 2000));
				source = await AdminService.getMDMAssetSource();
			}
			if (destroyed) return;
			assetSource = source;
			assetCatalog = await AdminService.listMDMAssets();
			assetsLoadError = undefined;
			const digestChanged =
				!source.isSyncing && Boolean(source.latestDigest) && source.latestDigest !== previousDigest;
			if (source.isSyncing) {
				checkNote = 'Still refreshing — check again in a moment.';
			} else if (digestChanged) {
				configuration = await AdminService.getMDMConfiguration(configuration.id);
				settingsOpen = configuration.assetDigest !== source.latestDigest;
			}
			// Re-initialize only when the fields could actually have changed, so an
			// uneventful check never discards in-progress edits.
			if (digestChanged || !hadCatalog) {
				initializeFromConfiguration();
			}
		} catch (error) {
			operationError = parseErrorContent(error).message;
		} finally {
			checkingForUpdates = false;
		}
	}

	function selectTarget(index: number) {
		selectedIndex = index;
		const target = targetOptions[index];
		if (target && typeof localStorage !== 'undefined') {
			localStorage.setItem(rememberedTargetKey, targetKey(target));
		}
		operationError = undefined;
	}

	// Choosing an installation method lands on its first operating system.
	function selectPlatform(platform: string) {
		const group = platformGroups.find((candidate) => candidate.platform === platform);
		if (group?.targets.length) {
			selectTarget(group.targets[0].index);
		}
	}

	async function handleSave() {
		const asset = latestAsset;
		if (readOnly || !asset || !catalogHasTargets || !formValid) return;

		saving = true;
		operationError = undefined;
		checkNote = undefined;
		try {
			configuration = await AdminService.updateMDMConfiguration(configuration.id, {
				assetDigest: asset.digest,
				values: submittedMDMValues(values)
			});
			values = editableMDMValues(asset.fields, configuration.values ?? {});
			settingsOpen = false;
		} catch (error) {
			const problem = parseErrorContent(error);
			operationError = problem.message;
			if (problem.status === 409) {
				await loadAssets();
				operationError = `${problem.message} The fields were reloaded; review and save again.`;
			}
		} finally {
			saving = false;
		}
	}

	async function handleDownload() {
		const artifact = selectedArtifact;
		if (!artifact || dirty) return;
		downloadLoading = true;
		operationError = undefined;
		try {
			const { blob, filename } = await AdminService.downloadMDMConfig(
				configuration.id,
				artifact.slug
			);
			saveBlob(blob, filename);
		} catch (error) {
			operationError = parseErrorContent(error).message;
		} finally {
			downloadLoading = false;
		}
	}
</script>

<section class="paper">
	<div class="flex flex-col gap-1.5">
		<div class="flex flex-wrap items-center justify-between gap-2">
			<div class="flex items-center gap-2.5">
				<h3 class="text-lg font-semibold">Install Obot Sentry</h3>
				{#if versionChip}
					<span class="badge badge-ghost badge-sm">{versionChip}</span>
				{/if}
				{#if configState === 'update'}
					<span class="badge badge-warning badge-sm">Update available</span>
				{/if}
			</div>
			<div class="flex items-center gap-2">
				{#if checkingForUpdates}
					<span class="text-muted-content text-xs">Checking…</span>
				{:else if checkNote}
					<span class="text-muted-content text-xs">{checkNote}</span>
				{:else if assetSource?.lastSyncTime}
					<span class="text-muted-content text-xs">
						checked {formatTimeAgo(assetSource.lastSyncTime).relativeTime}
					</span>
				{/if}
				{#if latestAsset}
					<IconButton
						class="btn-sm {settingsOpen ? 'text-primary' : ''}"
						tooltip={{ text: 'Agent settings' }}
						aria-expanded={settingsOpen}
						onclick={() => (settingsOpen = !settingsOpen)}
					>
						<Settings class="size-4" />
					</IconButton>
				{/if}
				{#if !readOnly}
					<IconButton
						class="btn-sm"
						tooltip={{ text: 'Check for updates' }}
						disabled={checkingForUpdates || saving}
						onclick={checkForUpdates}
					>
						<RefreshCw class="size-4 {checkingForUpdates ? 'animate-spin' : ''}" />
					</IconButton>
				{/if}
			</div>
		</div>

		<p class="text-muted-content text-sm font-light">
			Obot Sentry is a lightweight program that allows Obot to inventory and audit AI on enrolled
			devices. Follow the steps below to generate an installation package and deploy it to your
			devices.
		</p>

		{#if updateAvailable}
			<p class="text-warning text-xs">
				{latestVersion ? `v${latestVersion} is out` : 'A new release is out'} — your downloads were built
				with {versionChip ?? 'an older release'}. Save your settings to update.
			</p>
		{/if}

		{#if catalogError}
			<div class="notification-alert mt-1 flex items-start gap-2.5 p-2.5">
				<TriangleAlert class="size-4 shrink-0" />
				<div class="flex flex-1 flex-col gap-0.5">
					<span class="text-xs font-medium">
						{assetsLoadError ? "Couldn't load release info" : 'The last update check failed'}
					</span>
					<span class="text-muted-content text-xs break-all">{catalogError}</span>
				</div>
				{#if assetsLoadError}
					<button class="btn btn-secondary text-sm" onclick={loadAssets}>Retry</button>
				{/if}
			</div>
		{/if}

		{#if operationError}
			<p class="text-error text-xs break-all">{operationError}</p>
		{/if}
	</div>

	{#if assetsLoading}
		<Loading class="size-5 self-center" />
	{:else if !latestAsset && !catalogError}
		<div class="text-muted-content rounded-lg border border-dashed p-6 text-center text-sm">
			No Obot Sentry release available yet. Check for updates to fetch the latest.
		</div>
	{:else}
		{#if latestAsset && settingsOpen}
			<div class="border-base-300 flex flex-col gap-4 rounded-lg border p-4">
				{@render fieldsForm()}
				{#if !readOnly}
					<div class="flex justify-end">
						<button
							class="btn btn-primary flex items-center gap-2 text-sm"
							disabled={!catalogHasTargets || !formValid || !dirty || saving || checkingForUpdates}
							onclick={handleSave}
						>
							{#if saving}<Loading class="size-4" />{:else}<Save class="size-4" />{/if}
							Save
						</button>
					</div>
				{/if}
			</div>
		{/if}

		<div class="divide-base-300 dark:divide-base-400 flex flex-col divide-y">
			<div class="flex flex-wrap items-center justify-between gap-3 py-3">
				{@render setupStep(
					'1',
					'Generate an enrollment key',
					'Devices enroll with this server using this key — the instructions tell you where to put it.'
				)}
				{#if enrollmentKeyCount > 0}
					<span
						use:tooltip={'An enrollment key already exists'}
						role="img"
						aria-label="An enrollment key already exists"
					>
						<CircleCheck class="text-success size-5" />
					</span>
				{:else if !readOnly}
					<button class="btn btn-secondary btn-sm" onclick={() => onCreateEnrollmentKey?.()}>
						New Key
					</button>
				{/if}
			</div>

			{#if targetOptions.length > 0}
				<div class="flex flex-wrap items-center justify-between gap-3 py-3">
					{@render setupStep(
						'2',
						'Select your installation method',
						'How Obot Sentry is delivered to your devices'
					)}
					<div class="flex shrink-0 flex-wrap justify-end">
						{#each platformGroups as group (group.platform)}
							<button
								type="button"
								aria-pressed={selectedTarget?.platform === group.platform}
								class="border-b-2 px-3 py-1 text-sm text-nowrap transition-colors {selectedTarget?.platform ===
								group.platform
									? 'border-primary'
									: 'text-muted-content hover:border-primary/25 hover:text-base-content border-transparent'}"
								onclick={() => selectPlatform(group.platform)}
							>
								{group.platformLabel}
							</button>
						{/each}
					</div>
				</div>

				<div class="flex flex-wrap items-center justify-between gap-3 py-3">
					{@render setupStep(
						'3',
						'Select an operating system',
						'The operating system your devices run.'
					)}
					{#if selectedGroup}
						<div class="flex shrink-0 flex-wrap justify-end">
							{#each selectedGroup.targets as { option, index } (targetKey(option))}
								<button
									type="button"
									aria-pressed={selectedIndex === index}
									class="border-b-2 px-3 py-1 text-sm text-nowrap transition-colors {selectedIndex ===
									index
										? 'border-primary'
										: 'text-muted-content hover:border-primary/25 hover:text-base-content border-transparent'}"
									onclick={() => selectTarget(index)}
								>
									{option.osLabel}
								</button>
							{/each}
						</div>
					{/if}
				</div>

				<div class="flex flex-wrap items-center justify-between gap-3 py-3">
					{@render setupStep('4', 'Download the install artifacts', selectedTarget?.description)}
					{#if selectedArtifact && !dirty}
						<button
							type="button"
							class="text-link flex shrink-0 items-center gap-2 text-sm"
							disabled={downloadLoading || saving || checkingForUpdates}
							onclick={handleDownload}
						>
							obot-sentry-{selectedArtifact.slug}.zip
							{#if downloadLoading}<Loading class="size-4" />{:else}<Download class="size-4" />{/if}
						</button>
					{:else if downloadNote}
						<span class="text-warning shrink-0 text-xs">{downloadNote}</span>
					{/if}
				</div>

				<details class="collapse collapse-arrow rounded-none">
					<summary class="collapse-title min-h-0 px-0 py-3">
						<div class="flex items-center gap-3 pr-8">
							{@render setupStep(
								'5',
								'Follow the instructions to install obot-sentry',
								'The steps are specific to the installation method and operating system you selected.'
							)}
						</div>
					</summary>
					<div class="collapse-content px-0">
						<div class="pl-9">
							{#if selectedArtifact?.instructions}
								<div class="instructions milkdown-content" use:codeSnippetCopy>
									<!-- eslint-disable-next-line svelte/no-at-html-tags -- sanitized by toHTMLFromMarkdownWithNewTabLinks -->
									{@html toHTMLFromMarkdownWithNewTabLinks(selectedArtifact.instructions)}
								</div>
							{:else}
								<p class="text-muted-content text-sm">
									Instructions will appear once this download is generated.
								</p>
							{/if}
						</div>
					</div>
				</details>
			{/if}
		</div>
	{/if}
</section>

{@render enrollmentKeysSection?.()}

{#snippet setupStep(number: string, title: string, description?: string)}
	<div class="flex min-w-0 gap-3">
		<div
			class="bg-base-200 text-muted-content flex size-6 shrink-0 items-center justify-center rounded-full text-xs font-semibold"
		>
			{number}
		</div>
		<div class="min-w-0">
			<h4 class="text-sm font-semibold">{title}</h4>
			{#if description}
				<p class="text-muted-content text-sm">{description}</p>
			{/if}
		</div>
	</div>
{/snippet}

{#snippet fieldsForm()}
	<div class="flex flex-col gap-4">
		{#if formFields.length === 0}
			<p class="text-muted-content text-sm">This release has no editable values.</p>
		{/if}
		{#each formFields as [fieldName, field] (fieldName)}
			{@const problem = mdmFieldProblem(fieldName, field, values[fieldName], requiredFields)}
			<div class="flex flex-col gap-1 text-sm">
				<label for={`mdm-field-${fieldName}`} class="input-label">{field.title ?? fieldName}</label>
				{#if field.enum}
					<select
						id={`mdm-field-${fieldName}`}
						bind:value={values[fieldName]}
						disabled={readOnly}
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
						disabled={readOnly}
						onchange={(event) => (values[fieldName] = event.currentTarget.checked)}
					/>
				{:else if field.type === 'integer' || field.type === 'number'}
					<input
						id={`mdm-field-${fieldName}`}
						type="number"
						min={field.minimum}
						max={field.maximum}
						bind:value={values[fieldName]}
						disabled={readOnly}
						class="text-input-filled w-32"
					/>
				{:else}
					<input
						id={`mdm-field-${fieldName}`}
						type="text"
						bind:value={values[fieldName]}
						disabled={readOnly}
						class="text-input-filled"
					/>
				{/if}
				{#if problem}
					<span class="text-error text-xs">{problem}</span>
				{:else if field.description}
					<span class="input-description">{field.description}</span>
				{/if}
			</div>
		{/each}
	</div>
{/snippet}

<style>
	/* The rendered instructions sit inside the text-sm step list, so their
	   article-scale milkdown typography is stepped down to match. */
	.instructions {
		font-size: 0.875rem;
	}
	.instructions :global(p),
	.instructions :global(li),
	.instructions :global(code) {
		font-size: 0.875rem;
	}
	.instructions :global(h1) {
		font-size: 1rem;
	}
	.instructions :global(h2) {
		font-size: 0.9375rem;
	}
	.instructions :global(h3) {
		font-size: 0.875rem;
	}
	.instructions :global(pre) {
		margin-bottom: 1rem;
		overflow-x: auto;
		border-radius: 0.5rem;
		background-color: var(--color-base-200);
	}
	.instructions :global(pre code) {
		font-size: 0.8125rem;
		display: block;
		padding: 0;
		background-color: transparent;
		white-space: pre;
	}
	:global(.dark) .instructions :global(pre) {
		background-color: var(--color-base-300);
	}
	@layer base {
		.instructions :global(pre),
		.instructions :global(pre code),
		.instructions :global(pre code *) {
			color: var(--color-base-content) !important;
		}
	}
</style>
