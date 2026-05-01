<script lang="ts">
	import { page } from '$app/state';
	import Confirm from '$lib/components/Confirm.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import { PAGE_TRANSITION_DURATION } from '$lib/constants.js';
	import {
		AdminService,
		type ImagePullSecret,
		type ImagePullSecretCapability,
		type ImagePullSecretManifest,
		type ImagePullSecretTestResponse
	} from '$lib/services';
	import { profile } from '$lib/stores/index.js';
	import { goto } from '$lib/url';
	import CapabilityBanner from './CapabilityBanner.svelte';
	import ImagePullSecretForm from './ImagePullSecretForm.svelte';
	import ImagePullSecretStatusDialog from './ImagePullSecretStatusDialog.svelte';
	import ImagePullSecretTestDialog from './ImagePullSecretTestDialog.svelte';
	import ImagePullSecretsList from './ImagePullSecretsList.svelte';
	import { defaultForm, displayName, formFromSecret, type ImagePullSecretFormState } from './types';
	import { Plus } from 'lucide-svelte';
	import { untrack } from 'svelte';
	import { fly } from 'svelte/transition';

	const {
		data
	}: { data: { capability: ImagePullSecretCapability; imagePullSecrets: ImagePullSecret[] } } =
		$props();

	let capability = $state<ImagePullSecretCapability>(untrack(() => data.capability));
	let imagePullSecrets = $state<ImagePullSecret[]>(untrack(() => data.imagePullSecrets));
	let form = $state<ImagePullSecretFormState>(defaultForm('basic'));
	let showECRAdvanced = $state(false);

	let saving = $state(false);
	let testing = $state(false);
	let refreshing = $state(false);
	let statusLoading = $state(false);

	let testResult = $state<ImagePullSecretTestResponse>();
	let statusDetails = $state<ImagePullSecret>();

	let testError = $state('');
	let statusError = $state('');
	let refreshMessage = $state('');
	let testImage = $state('');
	let activeFormKey = $state('');
	let showRequired = $state(false);

	let deletingSecret = $state<ImagePullSecret>();
	let refreshingSecret = $state<ImagePullSecret>();
	let testingSecret = $state<ImagePullSecret>();
	let statusSecret = $state<ImagePullSecret>();

	let testDialog = $state<ReturnType<typeof ImagePullSecretTestDialog>>();
	let statusDialog = $state<ReturnType<typeof ImagePullSecretStatusDialog>>();

	let selectedId = $derived(page.url.searchParams.get('id'));
	let creatingSecret = $derived(page.url.searchParams.get('create') === 'true');
	let currentSecret = $derived(imagePullSecrets.find((item) => item.id === selectedId));
	let showForm = $derived(Boolean(creatingSecret || selectedId));
	let isReadonly = $derived(profile.current.isAdminReadonly?.());
	let mutationsDisabled = $derived(isReadonly || !capability.available);
	let title = $derived(
		creatingSecret
			? 'Create Image Pull Secret'
			: currentSecret
				? 'Edit Image Pull Secret'
				: 'Image Pull Secrets'
	);
	let requiredErrors = $derived(showRequired ? requiredFieldErrors() : {});
	const duration = PAGE_TRANSITION_DURATION;

	$effect(() => {
		const key = creatingSecret ? 'new' : selectedId ? `edit:${selectedId}` : '';
		if (key === activeFormKey) return;

		activeFormKey = key;
		testResult = undefined;
		testError = '';
		refreshMessage = '';
		showRequired = false;
		showECRAdvanced = false;
		form = currentSecret ? formFromSecret(currentSecret) : defaultForm('basic');
	});

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

	function inputFromForm(): ImagePullSecretManifest {
		const input: ImagePullSecretManifest = {
			enabled: form.enabled,
			type: form.type,
			displayName: form.displayName.trim()
		};

		if (form.type === 'basic') {
			input.basic = {
				server: form.server.trim(),
				username: form.username.trim()
			};
			if (form.password) {
				input.basic.password = form.password;
			}
		} else {
			input.ecr = {
				roleARN: form.roleARN.trim(),
				region: form.region.trim(),
				issuerURL: form.issuerURL.trim(),
				audience: form.audience.trim(),
				refreshSchedule: form.refreshSchedule.trim()
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

	async function saveSecret() {
		if (mutationsDisabled) return;
		showRequired = true;
		if (Object.keys(requiredFieldErrors()).length > 0) return;

		saving = true;
		testResult = undefined;
		testError = '';
		try {
			const input = inputFromForm();
			const saved = currentSecret
				? await AdminService.updateImagePullSecret(currentSecret.id, input)
				: await AdminService.createImagePullSecret(input);
			upsertSecret(saved);
			await refreshList();
			goto('/admin/image-pull-secrets', { replaceState: true, noScroll: true });
		} finally {
			saving = false;
		}
	}

	function openTestDialog(secret: ImagePullSecret) {
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
		if (!testingSecret || !testImage.trim() || mutationsDisabled) return;
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
		if (mutationsDisabled) return;
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

	function createNewSecret() {
		goto('/admin/image-pull-secrets?create=true', { noScroll: true });
	}

	function closeForm() {
		goto('/admin/image-pull-secrets', { replaceState: true, noScroll: true });
	}
</script>

<Layout {title} showBackButton={showForm} onBackButtonClick={closeForm}>
	<div
		class="h-full w-full"
		in:fly={{ x: 100, duration, delay: duration }}
		out:fly={{ x: -100, duration }}
	>
		{#if showForm}
			<ImagePullSecretForm
				bind:form
				bind:showECRAdvanced
				{capability}
				{currentSecret}
				{selectedId}
				{mutationsDisabled}
				{saving}
				{refreshing}
				{refreshMessage}
				{requiredErrors}
				onSave={saveSecret}
				onRefresh={refreshECR}
			/>
		{:else}
			<div class="flex flex-col gap-6">
				{#if !capability.available}
					<CapabilityBanner reason={capability.reason} />
				{/if}
				<ImagePullSecretsList
					{imagePullSecrets}
					{mutationsDisabled}
					{refreshing}
					onCreate={createNewSecret}
					onStatus={openStatusDialog}
					onTest={openTestDialog}
					onRefresh={(secret) => (refreshingSecret = secret)}
					onDelete={(secret) => (deletingSecret = secret)}
				/>
			</div>
		{/if}
	</div>

	{#snippet rightNavActions()}
		{#if !showForm && !mutationsDisabled}
			<button class="button-primary flex items-center gap-1 text-sm" onclick={createNewSecret}>
				<Plus class="size-4" />
				Create New Secret
			</button>
		{/if}
	{/snippet}
</Layout>

<Confirm
	msg={`Delete ${deletingSecret ? displayName(deletingSecret) : 'this image pull secret'}?`}
	show={Boolean(deletingSecret)}
	loading={saving}
	onsuccess={async () => {
		if (!deletingSecret) return;
		saving = true;
		try {
			await AdminService.deleteImagePullSecret(deletingSecret.id);
			imagePullSecrets = imagePullSecrets.filter((item) => item.id !== deletingSecret?.id);
			deletingSecret = undefined;
		} finally {
			saving = false;
		}
	}}
	oncancel={() => (deletingSecret = undefined)}
/>

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

<svelte:head>
	<title>Obot | Image Pull Secrets</title>
</svelte:head>
