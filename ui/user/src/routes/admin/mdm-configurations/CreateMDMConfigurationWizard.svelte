<script lang="ts">
	import { PAGE_TRANSITION_DURATION } from '$lib/constants';
	import { parseErrorContent } from '$lib/errors';
	import Loading from '$lib/icons/Loading.svelte';
	import {
		AdminService,
		type MDMAsset,
		type MDMAssetConfiguration,
		type MDMAssetSource,
		type MDMConfigurationCreateResponse
	} from '$lib/services';
	import { goto } from '$lib/url';
	import EnrollmentKeyRevealDialog from './EnrollmentKeyRevealDialog.svelte';
	import {
		defaultMDMValues,
		mdmFieldProblem,
		mdmTargetLabel,
		submittedMDMValues
	} from './platforms';
	import { TriangleAlert } from '@lucide/svelte';
	import { untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	interface Props {
		assetSource?: MDMAssetSource;
		assets: MDMAsset[];
		onCreate: (result: MDMConfigurationCreateResponse) => void;
		onCancel: () => void;
	}

	let { assetSource, assets, onCreate, onCancel }: Props = $props();

	let source = $state(untrack(() => assetSource));
	let assetCatalog = $state<MDMAsset[]>(untrack(() => assets));
	let selectedIndex = $state(-1);
	let values = $state<Record<string, unknown>>({});
	let name = $state('');
	let description = $state('');
	let loading = $state(false);
	let showValidation = $state(false);
	let createError = $state<string>();
	let created = $state<MDMConfigurationCreateResponse>();
	let revealedCredential = $state<string>();

	let latestAsset = $derived(assetCatalog.find((asset) => asset.digest === source?.latestDigest));
	let selected = $derived(latestAsset?.configurations[selectedIndex]);
	let catalogHasTargets = $derived((latestAsset?.configurations.length ?? 0) > 0);
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
	let nameError = $derived(showValidation && !name.trim());

	function targetLabel(configuration: MDMAssetConfiguration): string {
		return latestAsset ? mdmTargetLabel(latestAsset, configuration) : configuration.osLabel;
	}

	function handleTargetChange(event: Event) {
		selectedIndex = Number((event.currentTarget as HTMLSelectElement).value);
		values = selectedIndex >= 0 && latestAsset ? defaultMDMValues(latestAsset.fields) : {};
		createError = undefined;
	}

	async function handleCreate() {
		showValidation = true;
		createError = undefined;
		const asset = latestAsset;
		const target = selected;
		if (!name.trim() || (target && (!asset || !formValid))) return;

		loading = true;
		try {
			created = await AdminService.createMDMConfiguration({
				name: name.trim(),
				description: description.trim() || undefined,
				...(target && asset
					? {
							assetDigest: asset.digest,
							platform: target.platform,
							os: target.os,
							values: submittedMDMValues(values)
						}
					: {})
			});
			onCreate(created);
			revealedCredential = created.enrollmentCredential;
		} catch (error) {
			const problem = parseErrorContent(error);
			createError = problem.message;
			if (problem.status === 409) {
				selectedIndex = -1;
				values = {};
				try {
					[source, assetCatalog] = await Promise.all([
						AdminService.getMDMAssetSource(),
						AdminService.listMDMAssets()
					]);
					createError = `${problem.message} The target list was reloaded; choose a target again.`;
				} catch {
					createError = `${problem.message} Reload this page before retrying.`;
				}
			}
		} finally {
			loading = false;
		}
	}

	function viewCreatedConfiguration() {
		if (created) goto(`/admin/mdm-configurations/${created.id}`);
	}

	const duration = PAGE_TRANSITION_DURATION;
</script>

<div
	class="mx-auto flex h-full w-full max-w-3xl flex-col gap-6"
	out:fly={{ x: 100, duration }}
	in:fly={{ x: 100, delay: duration }}
>
	{#if created}
		<div class="paper flex flex-col gap-5 p-6">
			<div class="flex flex-col gap-1">
				<h3 class="text-lg font-semibold">Configuration created</h3>
				<p class="text-muted-content text-sm">
					{#if selected}
						{targetLabel(selected)} is pinned to this configuration. You can download its setup package
						from the configuration page.
					{:else}
						No deployment target was selected. You can choose one from the configuration page later.
					{/if}
				</p>
			</div>

			<div class="flex items-center justify-end gap-2">
				<button class="btn btn-secondary" onclick={viewCreatedConfiguration}>
					View Configuration
				</button>
				<button class="btn btn-primary" onclick={onCancel}>Done</button>
			</div>
		</div>
	{:else}
		<div class="paper flex flex-col gap-5 p-6">
			<div class="flex flex-col gap-1">
				<h3 class="text-lg font-semibold">Create an MDM configuration</h3>
				<p class="text-muted-content text-sm">
					Create the fleet and optionally pin a deployment target now.
				</p>
			</div>

			{#if !source || source.syncError || !catalogHasTargets}
				<div class="notification-alert flex items-start gap-3 p-3">
					<TriangleAlert class="size-5 shrink-0 text-warning" />
					<div class="flex flex-col gap-1">
						<span class="text-sm font-medium">
							{#if source?.syncError && catalogHasTargets}
								The latest asset refresh failed
							{:else if source?.syncError || !source}
								The deployment catalog is unavailable
							{:else}
								No deployment catalog is configured
							{/if}
						</span>
						<span class="text-muted-content text-xs">
							{#if source?.syncError && catalogHasTargets}
								The last successfully stored targets are still available. You can also set up the
								target later.
							{:else}
								You can create the configuration now and choose a target later.
							{/if}
						</span>
						{#if source?.syncError}
							<span class="text-muted-content text-xs break-all">{source.syncError}</span>
						{/if}
					</div>
				</div>
			{/if}

			<div class="grid gap-4 sm:grid-cols-2">
				<div class="flex flex-col gap-2">
					<label for="mdm-configuration-name" class="input-label">Name</label>
					<input
						id="mdm-configuration-name"
						type="text"
						bind:value={name}
						placeholder="e.g. Corporate laptops"
						class="text-input-filled"
					/>
					{#if nameError}<span class="text-error text-xs">Required.</span>{/if}
				</div>
				<div class="flex flex-col gap-2">
					<label for="mdm-configuration-description" class="input-label">
						Description (Optional)
					</label>
					<input
						id="mdm-configuration-description"
						type="text"
						bind:value={description}
						placeholder="Device fleet"
						class="text-input-filled"
					/>
				</div>
			</div>

			<div class="flex flex-col gap-2">
				<label for="mdm-deployment-target" class="input-label">Deployment Target</label>
				<select
					id="mdm-deployment-target"
					value={selectedIndex}
					onchange={handleTargetChange}
					class="text-input-filled"
				>
					<option value={-1}>Set up later (no deployment target)</option>
					{#each latestAsset?.configurations ?? [] as configuration, index (`${configuration.platform}/${configuration.os}`)}
						<option value={index}>{targetLabel(configuration)}</option>
					{/each}
				</select>
				<p class="input-description">
					The selected platform and OS assets are saved with this configuration.
				</p>
			</div>

			{#if selected}
				<div class="border-base-300 flex flex-col gap-4 border-t pt-5">
					<span class="text-sm font-medium">Deployment values</span>
					{@render fieldsForm()}
				</div>
			{/if}

			{#if createError}
				<div class="notification-alert flex items-start gap-2 p-3 text-sm">
					<TriangleAlert class="size-5 shrink-0 text-warning" />
					<span class="break-all">{createError}</span>
				</div>
			{/if}

			<div class="flex justify-end gap-2">
				<button class="btn btn-secondary" disabled={loading} onclick={onCancel}>Cancel</button>
				<button
					class="btn btn-primary flex items-center gap-2"
					disabled={loading}
					onclick={handleCreate}
				>
					{#if loading}<Loading class="size-4" />{/if}
					Create Configuration
				</button>
			</div>
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
				<label for={`mdm-create-field-${fieldName}`} class="text-muted">
					{field.title ?? fieldName}
				</label>
				{#if field.enum}
					<select
						id={`mdm-create-field-${fieldName}`}
						bind:value={values[fieldName]}
						class="text-input-filled w-fit"
					>
						<option value={undefined}>Select…</option>
						{#each field.enum as option (option)}<option value={option}>{option}</option>{/each}
					</select>
				{:else if field.type === 'boolean'}
					<input
						id={`mdm-create-field-${fieldName}`}
						type="checkbox"
						checked={values[fieldName] === true}
						onchange={(event) => (values[fieldName] = event.currentTarget.checked)}
					/>
				{:else if field.type === 'integer' || field.type === 'number'}
					<input
						id={`mdm-create-field-${fieldName}`}
						type="number"
						min={field.minimum}
						max={field.maximum}
						bind:value={values[fieldName]}
						class="text-input-filled w-32"
					/>
				{:else}
					<input
						id={`mdm-create-field-${fieldName}`}
						type="text"
						bind:value={values[fieldName]}
						class="text-input-filled"
					/>
				{/if}
				{#if showValidation && problem}
					<span class="text-error text-xs">{problem}</span>
				{:else if field.description}
					<span class="text-muted-content text-xs">{field.description}</span>
				{/if}
			</div>
		{/each}
	</div>
{/snippet}

<EnrollmentKeyRevealDialog
	credential={revealedCredential}
	onClose={() => (revealedCredential = undefined)}
/>
