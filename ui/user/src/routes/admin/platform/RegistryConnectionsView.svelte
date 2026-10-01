<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import {
		AdminService,
		type ImagePullSecret,
		type ImagePullSecretCapability,
		type ImagePullSecretManifest,
		type ImagePullSecretTestResponse,
		type ImagePullSecretType
	} from '$lib/services';
	import { canTest } from '$lib/services/admin/utils';
	import { errors, profile } from '$lib/stores/index.js';
	import { setUrlParamAndUpdateUrl } from '$lib/url';
	import { openUrl } from '$lib/utils.js';
	import CapabilityBanner from './CapabilityBanner.svelte';
	import ImagePullSecretForm from './ImagePullSecretForm.svelte';
	import ImagePullSecretStatusDialog from './ImagePullSecretStatusDialog.svelte';
	import ImagePullSecretTestDialog from './ImagePullSecretTestDialog.svelte';
	import ImagePullSecretsList from './ImagePullSecretsList.svelte';
	import { defaultForm, displayName, formFromSecret, type ImagePullSecretFormState } from './types';
	import { untrack } from 'svelte';

	const LIST_PATH = '/admin/platform?view=settings';
	const draftPrefix = 'draft-';

	type DraftCreate = {
		id: string;
		form: ImagePullSecretFormState;
	};

	interface Props {
		capability: ImagePullSecretCapability;
		imagePullSecrets: ImagePullSecret[];
		dirty?: boolean;
		saving?: boolean;
	}

	let {
		capability = $bindable(),
		imagePullSecrets = $bindable(),
		dirty = $bindable(false),
		saving: settingsSaving = false
	}: Props = $props();

	const initialCreate = untrack(() => page.url.searchParams.get('create') === 'true');
	const initialSecret = untrack(() => {
		if (initialCreate) return undefined;
		const id = page.url.searchParams.get('id');
		return id ? imagePullSecrets.find((item) => item.id === id) : undefined;
	});
	const initialId = initialSecret?.id ?? null;

	let mode = $state<'closed' | 'create' | 'edit'>(
		initialCreate ? 'create' : initialId ? 'edit' : 'closed'
	);
	let editingId = $state<string | null>(initialId);
	let form = $state<ImagePullSecretFormState>(
		untrack(() => (initialSecret ? formFromSecret(initialSecret) : defaultForm('basic')))
	);
	let showECRAdvanced = $state(false);
	let baseline = $state<ImagePullSecret[]>(untrack(() => cloneSecrets(imagePullSecrets)));
	let pendingCreates = $state<DraftCreate[]>([]);
	let pendingEdits = $state<Record<string, ImagePullSecretFormState>>({});
	let pendingDeletes = $state<string[]>([]);
	let editorDialog = $state<ReturnType<typeof ResponsiveDialog>>();

	let requestPending = $state(false);
	let testing = $state(false);
	let refreshing = $state(false);
	let statusLoading = $state(false);

	let testResult = $state<ImagePullSecretTestResponse>();
	let statusDetails = $state<ImagePullSecret>();

	let testError = $state('');
	let statusError = $state('');
	let refreshMessage = $state('');
	let testImage = $state('');
	let showRequired = $state(false);

	let refreshingSecret = $state<ImagePullSecret>();
	let testingSecret = $state<ImagePullSecret>();
	let statusSecret = $state<ImagePullSecret>();

	let testDialog = $state<ReturnType<typeof ImagePullSecretTestDialog>>();
	let statusDialog = $state<ReturnType<typeof ImagePullSecretStatusDialog>>();

	let currentSecret = $derived(
		mode === 'edit' && editingId && !editingId.startsWith(draftPrefix)
			? baseline.find((item) => item.id === editingId)
			: undefined
	);
	let editingPersisted = $derived(Boolean(currentSecret));
	let isReadonly = $derived(profile.current.isAdminReadonly?.());
	let mutationsDisabled = $derived(isReadonly || !capability.available);
	let formLocked = $derived(mutationsDisabled || settingsSaving || requestPending);
	let requiredErrors = $derived(showRequired ? requiredFieldErrors() : {});
	let canStage = $derived(Object.keys(requiredFieldErrors()).length === 0);
	let isDirty = $derived(
		pendingCreates.length > 0 || Object.keys(pendingEdits).length > 0 || pendingDeletes.length > 0
	);
	let unsavedIds = $derived(
		new Set([...pendingCreates.map((draft) => draft.id), ...Object.keys(pendingEdits)])
	);
	let showRefresh = $derived(
		Boolean(
			currentSecret &&
			form.type === 'ecr' &&
			!pendingEdits[currentSecret.id] &&
			JSON.stringify(formFromSecret(currentSecret)) === JSON.stringify(form)
		)
	);
	let rows = $derived([
		...pendingCreates.map((draft) => secretFromDraft(draft)),
		...baseline
			.filter((secret) => !pendingDeletes.includes(secret.id))
			.map((secret) => {
				const edit = pendingEdits[secret.id];
				return edit ? secretFromForm(secret, edit) : secret;
			})
	]);

	$effect(() => {
		if (dirty !== isDirty) dirty = isDirty;
	});

	$effect(() => {
		const incoming = imagePullSecrets;
		if (
			pendingCreates.length > 0 ||
			Object.keys(pendingEdits).length > 0 ||
			pendingDeletes.length > 0
		) {
			return;
		}
		baseline = cloneSecrets(incoming);
	});

	$effect(() => {
		if (!editorDialog || mode === 'closed') return;
		editorDialog.open();
	});

	function openEditor(
		nextMode: 'create' | 'edit',
		secret?: ImagePullSecret,
		type: ImagePullSecretType = 'basic'
	) {
		mode = nextMode;
		editingId = secret?.id ?? null;
		showRequired = false;
		showECRAdvanced = false;
		refreshMessage = '';
		form = secret ? formForSecret(secret) : defaultForm(type);
		editorDialog?.open();
	}

	function openCreateForm(type: ImagePullSecretType) {
		if (mutationsDisabled) return;
		openEditor('create', undefined, type);
	}

	function closeEditor() {
		if (mode === 'create') {
			setUrlParamAndUpdateUrl(page.url, 'create', null);
		} else if (mode === 'edit') {
			setUrlParamAndUpdateUrl(page.url, 'id', null);
		}
		mode = 'closed';
		editingId = null;
		showRequired = false;
	}

	export function reset() {
		pendingCreates = [];
		pendingEdits = {};
		pendingDeletes = [];
		showRequired = false;
		showECRAdvanced = false;
		refreshMessage = '';
		mode = 'closed';
		editingId = null;
		form = defaultForm('basic');
		editorDialog?.close();
	}

	export function validate() {
		return true;
	}

	function cloneSecrets(items: ImagePullSecret[]) {
		return items.map((item) => ({
			...item,
			manifest: {
				...item.manifest,
				basic: item.manifest.basic ? { ...item.manifest.basic } : undefined,
				ecr: item.manifest.ecr ? { ...item.manifest.ecr } : undefined
			},
			status: item.status ? { ...item.status } : undefined
		}));
	}

	function formForSecret(secret?: ImagePullSecret) {
		if (!secret) return defaultForm('basic');
		const draft = pendingCreates.find((item) => item.id === secret.id);
		if (draft) return { ...draft.form };
		if (pendingEdits[secret.id]) return { ...pendingEdits[secret.id] };
		const original = baseline.find((item) => item.id === secret.id) ?? secret;
		return formFromSecret(original);
	}

	function secretFromForm(
		secret: ImagePullSecret,
		nextForm: ImagePullSecretFormState
	): ImagePullSecret {
		return {
			...secret,
			manifest: inputFromForm(nextForm),
			status: {
				...secret.status,
				passwordConfigured: secret.status?.passwordConfigured || Boolean(nextForm.password)
			}
		};
	}

	function secretFromDraft(draft: DraftCreate): ImagePullSecret {
		return secretFromForm(
			{
				id: draft.id,
				manifest: inputFromForm(draft.form),
				status: draft.form.password ? { passwordConfigured: true } : undefined
			},
			draft.form
		);
	}

	function stageDelete(secret: ImagePullSecret) {
		if (formLocked) return;
		if (secret.id.startsWith(draftPrefix)) {
			pendingCreates = pendingCreates.filter((draft) => draft.id !== secret.id);
			return;
		}
		if (pendingDeletes.includes(secret.id)) return;
		pendingDeletes = [...pendingDeletes, secret.id];
		delete pendingEdits[secret.id];
	}

	function stageSecret() {
		if (formLocked || !canStage) {
			showRequired = true;
			return;
		}

		const staged = { ...form };
		if (!editingId || editingId.startsWith(draftPrefix)) {
			const draft: DraftCreate = {
				id: editingId ?? `${draftPrefix}${crypto.randomUUID()}`,
				form: staged
			};
			pendingCreates = editingId
				? pendingCreates.map((item) => (item.id === draft.id ? draft : item))
				: [draft, ...pendingCreates];
		} else {
			const original = baseline.find((secret) => secret.id === editingId);
			if (original && JSON.stringify(formFromSecret(original)) === JSON.stringify(staged)) {
				delete pendingEdits[editingId];
			} else {
				pendingEdits = { ...pendingEdits, [editingId]: staged };
			}
		}
		editorDialog?.close();
	}

	function requiredFieldErrors(): Record<string, string> {
		const errors: Record<string, string> = {};

		if (form.type === 'basic') {
			if (!form.server.trim()) errors.server = 'Registry Server is required';
			if (!form.username.trim()) errors.username = 'Username is required';
			if (!currentSecret?.status?.passwordConfigured && !form.password) {
				errors.password = 'Password is required';
			}
		} else {
			if (!form.roleARN.trim()) errors.roleARN = 'Role ARN is required';
			if (!form.region.trim()) errors.region = 'Region is required';
		}

		return errors;
	}

	function inputFromForm(source: ImagePullSecretFormState = form): ImagePullSecretManifest {
		const input: ImagePullSecretManifest = {
			enabled: source.enabled,
			type: source.type,
			displayName: source.displayName.trim()
		};

		if (source.type === 'basic') {
			input.basic = {
				server: source.server.trim(),
				username: source.username.trim()
			};
			if (source.password) {
				input.basic.password = source.password;
			}
		} else {
			input.ecr = {
				roleARN: source.roleARN.trim(),
				region: source.region.trim(),
				issuerURL: source.issuerURL.trim(),
				audience: source.audience.trim(),
				refreshSchedule: source.refreshSchedule.trim()
			};
		}

		return input;
	}

	function upsertSecret(secret: ImagePullSecret) {
		const index = imagePullSecrets.findIndex((item) => item.id === secret.id);
		if (index === -1) {
			imagePullSecrets = [secret, ...imagePullSecrets];
		} else {
			imagePullSecrets = imagePullSecrets.map((item) => (item.id === secret.id ? secret : item));
		}
	}

	async function refreshList() {
		const [nextCapability, nextSecrets] = await Promise.all([
			AdminService.getImagePullSecretCapability(),
			AdminService.listImagePullSecrets()
		]);
		capability = nextCapability;
		imagePullSecrets = nextSecrets;
	}

	export async function save() {
		if (!isDirty || mutationsDisabled || requestPending) return false;

		requestPending = true;
		const failedCreates: DraftCreate[] = [];
		const failedEdits: Record<string, ImagePullSecretFormState> = {};
		const failedDeletes: string[] = [];
		const created: ImagePullSecret[] = [];
		const deletedIds: string[] = [];
		// eslint-disable-next-line svelte/prefer-svelte-reactivity
		const edited = new Map<string, ImagePullSecret>();
		try {
			await Promise.all([
				...pendingCreates.map(async (draft) => {
					try {
						created.push(
							await AdminService.createImagePullSecret(inputFromForm(draft.form), {
								dontLogErrors: true
							})
						);
					} catch (error) {
						failedCreates.push(draft);
						errors.append(
							error instanceof Error ? error.message : 'Unable to save image pull secret'
						);
					}
				}),
				...Object.entries(pendingEdits)
					.filter(([id]) => !pendingDeletes.includes(id))
					.map(async ([id, edit]) => {
						try {
							edited.set(
								id,
								await AdminService.updateImagePullSecret(id, inputFromForm(edit), {
									dontLogErrors: true
								})
							);
						} catch (error) {
							failedEdits[id] = edit;
							errors.append(
								error instanceof Error ? error.message : 'Unable to save image pull secret'
							);
						}
					}),
				...pendingDeletes.map(async (id) => {
					try {
						await AdminService.deleteImagePullSecret(id, { dontLogErrors: true });
						deletedIds.push(id);
					} catch (error) {
						failedDeletes.push(id);
						errors.append(
							error instanceof Error ? error.message : 'Unable to delete image pull secret'
						);
					}
				})
			]);

			const next = [
				...created,
				...baseline
					.filter((secret) => !deletedIds.includes(secret.id))
					.map((secret) => edited.get(secret.id) ?? secret)
			];
			baseline = next;
			imagePullSecrets = next;
			pendingCreates = failedCreates;
			pendingEdits = failedEdits;
			pendingDeletes = failedDeletes;
			return (
				failedCreates.length === 0 &&
				Object.keys(failedEdits).length === 0 &&
				failedDeletes.length === 0
			);
		} finally {
			requestPending = false;
		}
	}

	function openTestDialog(secret: ImagePullSecret) {
		if (!canTest(secret) || unsavedIds.has(secret.id)) return;
		testingSecret = secret;
		testImage = '';
		testResult = undefined;
		testError = '';
		testDialog?.open();
	}

	function resetTestDialog() {
		testingSecret = undefined;
		testImage = '';
		testResult = undefined;
		testError = '';
	}

	async function testSecret() {
		if (
			!testingSecret ||
			!canTest(testingSecret) ||
			unsavedIds.has(testingSecret.id) ||
			!testImage.trim() ||
			mutationsDisabled
		)
			return;
		testing = true;
		testResult = undefined;
		testError = '';
		try {
			testResult = await AdminService.testImagePullSecret(
				testingSecret.id,
				{ image: testImage.trim() },
				{ dontLogErrors: true }
			);
		} catch (err) {
			testError = err instanceof Error ? err.message : 'Image pull secret test failed';
		} finally {
			testing = false;
		}
	}

	async function openStatusDialog(secret: ImagePullSecret) {
		statusSecret = secret;
		statusDetails = undefined;
		statusError = '';
		statusLoading = true;
		statusDialog?.open();
		try {
			const details = await AdminService.getImagePullSecret(secret.id, {
				dontLogErrors: true
			});
			statusDetails = details;
			upsertSecret(details);
		} catch (err) {
			statusError = err instanceof Error ? err.message : 'Failed to load image pull secret status';
		} finally {
			statusLoading = false;
		}
	}

	function resetStatusDialog() {
		statusSecret = undefined;
		statusDetails = undefined;
		statusError = '';
		statusLoading = false;
	}

	async function refreshECR(secret: ImagePullSecret) {
		if (mutationsDisabled || unsavedIds.has(secret.id)) return;
		refreshing = true;
		refreshMessage = '';
		try {
			const response = await AdminService.refreshImagePullSecret(secret.id);
			refreshMessage = response.message ?? 'Refresh started';
			await refreshList();
		} finally {
			refreshing = false;
		}
	}

	function editSecret(secret: ImagePullSecret, isCtrlClick: boolean) {
		if (isCtrlClick) {
			openUrl(`${LIST_PATH}&id=${secret.id}`, true);
			return;
		}
		openEditor('edit', secret);
	}
</script>

<div class="flex flex-col gap-6">
	{#if !capability.available}
		<CapabilityBanner reason={capability.reason} />
	{/if}
	<ImagePullSecretsList
		imagePullSecrets={rows}
		{mutationsDisabled}
		{refreshing}
		onCreate={openCreateForm}
		onEdit={editSecret}
		onStatus={openStatusDialog}
		onTest={openTestDialog}
		onRefresh={(secret) => (refreshingSecret = secret)}
		onDelete={stageDelete}
		{unsavedIds}
	/>
</div>

<ResponsiveDialog
	bind:this={editorDialog}
	title={mode === 'edit'
		? `Edit ${currentSecret ? displayName(currentSecret) : form.type === 'basic' ? 'Basic Secret' : 'ECR Secret'}`
		: `Add ${form.type === 'basic' ? 'Basic Secret' : 'ECR Secret'}`}
	class="w-full md:max-w-4xl"
	onClose={closeEditor}
>
	{#if mode !== 'closed'}
		<div class="flex flex-col gap-4 p-4 md:p-0">
			<ImagePullSecretForm
				bind:form
				bind:showECRAdvanced
				{capability}
				{currentSecret}
				selectedId={editingPersisted ? editingId : null}
				mutationsDisabled={formLocked}
				saving={requestPending}
				{refreshing}
				{refreshMessage}
				{requiredErrors}
				hideSubmit
				{showRefresh}
				onSave={stageSecret}
				onRefresh={refreshECR}
			/>
			<div class="flex justify-end gap-2">
				<button
					type="button"
					class="btn btn-secondary text-sm"
					disabled={formLocked}
					onclick={() => editorDialog?.close()}
				>
					Cancel
				</button>
				<button
					type="button"
					class="btn btn-primary text-sm"
					disabled={formLocked || !canStage}
					onclick={stageSecret}
				>
					{mode === 'edit' ? 'Update' : 'Add'}
				</button>
			</div>
		</div>
	{/if}
</ResponsiveDialog>

<Confirm
	title="Refresh Image Pull Secret"
	type="info"
	msg={`Refresh ${refreshingSecret ? displayName(refreshingSecret) : 'this image pull secret'}?`}
	note="This requests an immediate refresh of the generated ECR image pull secret."
	show={Boolean(refreshingSecret)}
	loading={refreshing}
	submitText="Refresh"
	onsuccess={async () => {
		if (!refreshingSecret) return;
		await refreshECR(refreshingSecret);
		refreshingSecret = undefined;
	}}
	oncancel={() => (refreshingSecret = undefined)}
/>

<ImagePullSecretStatusDialog
	bind:this={statusDialog}
	secret={statusSecret}
	details={statusDetails}
	loading={statusLoading}
	error={statusError}
	onClose={resetStatusDialog}
/>

<ImagePullSecretTestDialog
	bind:this={testDialog}
	secret={testingSecret}
	bind:testImage
	{testing}
	{testResult}
	{testError}
	onTest={testSecret}
	onClose={resetTestDialog}
/>
