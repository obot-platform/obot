<script lang="ts">
	import { AdminService } from '$lib/services';
	import Dropdown from '$lib/components/tasks/Dropdown.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import { AlertTriangle, LoaderCircle } from 'lucide-svelte';
	import type { StorageCredentials } from '$lib/services/admin/types';
	import Success from '$lib/components/Success.svelte';
	import { onMount } from 'svelte';

	interface Props {
		onCancel: () => void;
		onSubmit: () => void;
	}

	let { onCancel, onSubmit }: Props = $props();

	// Authentication method selection
	let useWorkloadIdentity = $state(false);

	// Form state
	let form = $state<StorageCredentials>({
		provider: 's3',
		useWorkloadIdentity: false,
		s3Config: {
			region: '',
			accessKeyID: '',
			secretAccessKey: '',
			sessionToken: ''
		},
		gcsConfig: {
			serviceAccountJSON: ''
		},
		azureConfig: {
			storageAccount: '',
			clientID: '',
			tenantID: '',
			clientSecret: ''
		},
		customS3Config: {
			endpoint: '',
			region: '',
			accessKeyID: '',
			secretAccessKey: ''
		}
	});

	let saving = $state(false);
	let testing = $state(false);
	let loading = $state(true);
	let error = $state('');
	let testResult = $state<{ success: boolean; message: string } | null>(null);
	let existingCredentials = $state<StorageCredentials | null>(null);

	// Load existing storage credentials on mount
	onMount(async () => {
		try {
			existingCredentials = await AdminService.getStorageCredentials();
			if (existingCredentials && existingCredentials.provider) {
				// Populate form with existing data
				form.provider = existingCredentials.provider;
				form.useWorkloadIdentity = existingCredentials.useWorkloadIdentity || false;

				if (existingCredentials.s3Config) {
					form.s3Config = {
						region: existingCredentials.s3Config.region || '',
						accessKeyID: existingCredentials.s3Config.accessKeyID || '',
						secretAccessKey: existingCredentials.s3Config.secretAccessKey || '',
						sessionToken: existingCredentials.s3Config.sessionToken || ''
					};

					form.gcsConfig = undefined;
					form.azureConfig = undefined;
					form.customS3Config = undefined;
					useWorkloadIdentity = existingCredentials.useWorkloadIdentity;
				} else if (existingCredentials.gcsConfig) {
					form.gcsConfig = {
						serviceAccountJSON: existingCredentials.gcsConfig.serviceAccountJSON || ''
					};
					form.s3Config = undefined;
					form.azureConfig = undefined;
					form.customS3Config = undefined;
					useWorkloadIdentity = existingCredentials.useWorkloadIdentity;
				} else if (existingCredentials.azureConfig) {
					form.azureConfig = {
						storageAccount: existingCredentials.azureConfig.storageAccount || '',
						clientID: existingCredentials.azureConfig.clientID || '',
						tenantID: existingCredentials.azureConfig.tenantID || '',
						clientSecret: existingCredentials.azureConfig.clientSecret || ''
					};
					useWorkloadIdentity = existingCredentials.useWorkloadIdentity;
				} else if (existingCredentials.customS3Config) {
					form.customS3Config = {
						endpoint: existingCredentials.customS3Config.endpoint || '',
						region: existingCredentials.customS3Config.region || '',
						accessKeyID: existingCredentials.customS3Config.accessKeyID || '',
						secretAccessKey: existingCredentials.customS3Config.secretAccessKey || ''
					};
					form.useWorkloadIdentity = false;
					form.s3Config = undefined;
					form.gcsConfig = undefined;
					form.azureConfig = undefined;
					useWorkloadIdentity = false;
				}
			}
		} catch (error) {
			// Ignore errors - likely means no credentials are configured yet
			console.error('Failed to get storage credentials:', error);
		} finally {
			loading = false;
		}
	});

	async function handleSubmit() {
		try {
			saving = true;
			error = '';

			// Validate required fields based on provider and auth method
			if (!useWorkloadIdentity) {
				if (form.provider === 's3') {
					if (!form.s3Config?.region) {
						throw new Error('Region is required for S3');
					}
					if (!form.s3Config?.accessKeyID) {
						throw new Error('Access Key ID is required for S3');
					}
					if (!form.s3Config?.secretAccessKey && !existingCredentials?.s3Config) {
						throw new Error('Secret Access Key is required for S3');
					}
				} else if (form.provider === 'gcs') {
					if (!form.gcsConfig?.serviceAccountJSON && !existingCredentials?.gcsConfig) {
						throw new Error('Service Account JSON is required for GCS');
					}
				} else if (form.provider === 'azure') {
					if (!form.azureConfig?.storageAccount) {
						throw new Error('Storage Account is required for Azure');
					}
					if (!form.azureConfig?.clientID) {
						throw new Error('Client ID is required for Azure');
					}
					if (!form.azureConfig?.tenantID) {
						throw new Error('Tenant ID is required for Azure');
					}
					if (!form.azureConfig?.clientSecret && !existingCredentials?.azureConfig) {
						throw new Error('Client Secret is required for Azure');
					}
				} else if (form.provider === 'custom') {
					if (!form.customS3Config?.endpoint) {
						throw new Error('Endpoint is required for Custom S3');
					}
					if (!form.customS3Config?.region) {
						throw new Error('Region is required for Custom S3');
					}
					if (!form.customS3Config?.accessKeyID) {
						throw new Error('Access Key ID is required for Custom S3');
					}
					if (!form.customS3Config?.secretAccessKey && !existingCredentials?.customS3Config) {
						throw new Error('Secret Access Key is required for Custom S3');
					}
				}
			}

			const request = {
				...form,
				useWorkloadIdentity
			};

			await AdminService.configureStorageCredentials(request);
			onSubmit();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to configure credentials';
		} finally {
			saving = false;
		}
	}

	async function handleTest() {
		try {
			testing = true;
			error = '';
			testResult = null;

			// Prepare test request - clean masked fields
			const request = {
				...form,
				useWorkloadIdentity
			};

			// Clean masked fields before sending
			if (request.s3Config) {
				request.s3Config = {
					...request.s3Config,
					accessKeyID: request.s3Config.accessKeyID,
					secretAccessKey: request.s3Config.secretAccessKey
				};
				if (useWorkloadIdentity) {
					request.s3Config.accessKeyID = '';
					request.s3Config.secretAccessKey = '';
				}
			}

			if (request.gcsConfig) {
				request.gcsConfig = {
					...request.gcsConfig,
					serviceAccountJSON: request.gcsConfig.serviceAccountJSON
				};
				if (useWorkloadIdentity) {
					request.gcsConfig.serviceAccountJSON = '';
				}
			}

			if (request.azureConfig) {
				request.azureConfig = {
					...request.azureConfig,
					clientID: request.azureConfig.clientID,
					tenantID: request.azureConfig.tenantID,
					clientSecret: request.azureConfig.clientSecret
				};
				if (useWorkloadIdentity) {
					request.azureConfig.clientID = '';
					request.azureConfig.tenantID = '';
					request.azureConfig.clientSecret = '';
				}
			}

			if (request.customS3Config) {
				request.customS3Config = {
					...request.customS3Config,
					accessKeyID: request.customS3Config.accessKeyID,
					secretAccessKey: request.customS3Config.secretAccessKey
				};
			}

			const result = await AdminService.testStorageCredentials(request);
			testResult = result as { success: boolean; message: string } | null;
		} catch (err) {
			testResult = {
				success: false,
				message: err instanceof Error ? err.message : 'Test failed'
			};
		} finally {
			testing = false;
		}
	}
</script>

{#if loading}
	<div class="dark:bg-surface2 rounded-md bg-white p-6 shadow-sm">
		<div class="flex items-center justify-center py-8">
			<LoaderCircle class="size-6 animate-spin text-blue-500" />
			<span class="ml-2 text-sm text-gray-600">Loading storage credentials...</span>
		</div>
	</div>
{:else}
	<div class="dark:bg-surface2 rounded-md bg-white p-6 shadow-sm">
		<form
			class="space-y-8"
			onsubmit={(e) => {
				e.preventDefault();
				handleSubmit();
			}}
		>
			<!-- Provider Selection -->
			<div class="space-y-4">
				<h3 class="text-lg font-semibold">Storage Provider</h3>
				<div class="flex flex-col gap-1">
					<label class="text-sm font-medium" for="storage-provider">Provider</label>
					<div>
						<Dropdown
							class="w-full md:w-1/3"
							values={{
								s3: 'Amazon S3',
								gcs: 'Google Cloud Storage',
								azure: 'Azure Blob Storage',
								custom: 'Custom S3 Compatible'
							}}
							selected={form.provider}
							onSelected={(value) => {
								form.provider = value;
								testResult = null; // Clear test result when provider changes

								// Clear other provider configs and initialize selected one
								if (value === 's3') {
									form.gcsConfig = undefined;
									form.azureConfig = undefined;
									form.customS3Config = undefined;
									form.s3Config = {
										region: existingCredentials?.s3Config?.region || '',
										accessKeyID: existingCredentials?.s3Config?.accessKeyID || '',
										secretAccessKey: existingCredentials?.s3Config?.secretAccessKey || '',
										sessionToken: existingCredentials?.s3Config?.sessionToken || ''
									};
									useWorkloadIdentity = false;
								} else if (value === 'gcs') {
									form.s3Config = undefined;
									form.azureConfig = undefined;
									form.customS3Config = undefined;
									form.gcsConfig = {
										serviceAccountJSON: existingCredentials?.gcsConfig?.serviceAccountJSON || ''
									};
								} else if (value === 'azure') {
									form.s3Config = undefined;
									form.gcsConfig = undefined;
									form.customS3Config = undefined;
									form.azureConfig = {
										storageAccount: existingCredentials?.azureConfig?.storageAccount || '',
										clientID: existingCredentials?.azureConfig?.clientID || '',
										tenantID: existingCredentials?.azureConfig?.tenantID || '',
										clientSecret: existingCredentials?.azureConfig?.clientSecret || ''
									};
								} else if (value === 'custom') {
									form.s3Config = undefined;
									form.gcsConfig = undefined;
									form.azureConfig = undefined;
									form.customS3Config = {
										endpoint: existingCredentials?.customS3Config?.endpoint || '',
										region: existingCredentials?.customS3Config?.region || '',
										accessKeyID: existingCredentials?.customS3Config?.accessKeyID || '',
										secretAccessKey: existingCredentials?.customS3Config?.secretAccessKey || ''
									};
									useWorkloadIdentity = false;
								}
							}}
						/>
					</div>
				</div>
			</div>

			<div class="space-y-4">
				{#if form.provider === 's3' && form.s3Config}
					<div class="grid grid-cols-1 gap-6 md:grid-cols-2">
						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="region">Region</label>
							<input
								class="text-input-filled"
								id="region"
								bind:value={form.s3Config.region}
								placeholder="e.g. us-east-1"
							/>
						</div>
					</div>
				{/if}
				{#if form.provider === 'azure' && form.azureConfig}
					<div class="grid grid-cols-1 gap-6 md:grid-cols-2">
						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="storage-account">Storage Account</label>
							<input
								class="text-input-filled"
								id="storage-account"
								bind:value={form.azureConfig.storageAccount}
								placeholder="my-storage-account"
							/>
						</div>
					</div>
				{/if}
			</div>

			<!-- Authentication Method -->
			{#if form.provider !== 'custom'}
				<div class="space-y-4">
					<h3 class="text-lg font-semibold">Authentication Method</h3>
					<div class="flex flex-col gap-4">
						<div class="flex items-center justify-between">
							<div class="flex flex-col gap-1">
								<label class="text-sm font-medium" for="auth-method"
									>Use credential associated with Obot</label
								>
							</div>
							<Toggle
								checked={useWorkloadIdentity}
								onChange={(checked) => (useWorkloadIdentity = checked)}
								label={useWorkloadIdentity ? 'Use workload identity' : 'Configure keys manually'}
							/>
						</div>
						{#if useWorkloadIdentity}
							<div class="rounded-md bg-blue-50 p-4 dark:bg-blue-950/50">
								<p class="text-sm text-blue-700 dark:text-blue-300">
									Using existing workload identity from Obot. No manual credentials required.
								</p>
							</div>
						{/if}
					</div>
				</div>
			{/if}

			<!-- Credentials -->
			{#if !useWorkloadIdentity}
				<div class="space-y-4">
					<h3 class="text-lg font-semibold">Credentials</h3>

					{#if form.provider === 's3' && form.s3Config}
						<div class="space-y-4">
							<div class="flex flex-col gap-1">
								<label class="text-sm font-medium" for="access-key">Access Key ID</label>
								<input
									name="access-key"
									class="text-input-filled"
									bind:value={form.s3Config.accessKeyID}
								/>
							</div>
							<div class="flex flex-col gap-1">
								<label class="text-sm font-medium" for="secret-key">Secret Access Key</label>
								<SensitiveInput
									name="secret-key"
									bind:value={form.s3Config.secretAccessKey}
									placeholder={existingCredentials?.s3Config
										? '••••••••••••••••••••••••••••••••••••••••'
										: ''}
									hideReveal
								/>
							</div>
						</div>
					{:else if form.provider === 'gcs' && form.gcsConfig}
						<div class="flex flex-col gap-1">
							<label class="text-sm font-medium" for="service-account">Service Account JSON</label>
							<SensitiveInput
								name="service-account-json"
								bind:value={form.gcsConfig.serviceAccountJSON}
								textarea
								placeholder={existingCredentials?.gcsConfig
									? '••••••••••••••••••••••••••••••••••••••••'
									: ''}
								hideReveal
							/>
							<p class="text-xs text-gray-500">Complete JSON key file for the service account</p>
						</div>
					{:else if form.provider === 'azure' && form.azureConfig}
						<div class="space-y-4">
							<div class="flex flex-col gap-1">
								<label class="text-sm font-medium" for="azure-client-id">Client ID</label>
								<input
									name="azure-client-id"
									class="text-input-filled"
									bind:value={form.azureConfig.clientID}
								/>
							</div>
							<div class="flex flex-col gap-1">
								<label class="text-sm font-medium" for="azure-tenant-id">Tenant ID</label>
								<input
									name="azure-tenant-id"
									class="text-input-filled"
									bind:value={form.azureConfig.tenantID}
								/>
							</div>
							<div class="flex flex-col gap-1">
								<label class="text-sm font-medium" for="azure-client-secret">Client Secret</label>
								<SensitiveInput
									name="azure-client-secret"
									bind:value={form.azureConfig.clientSecret}
									hideReveal
									placeholder={existingCredentials?.azureConfig
										? '••••••••••••••••••••••••••••••••••••••••'
										: ''}
								/>
							</div>
						</div>
					{:else if form.provider === 'custom' && form.customS3Config}
						<div class="space-y-4">
							<div class="flex flex-col gap-1">
								<label class="text-sm font-medium" for="custom-endpoint">Endpoint</label>
								<input
									class="text-input-filled"
									id="custom-endpoint"
									bind:value={form.customS3Config.endpoint}
									placeholder="https://s3.example.com"
								/>
							</div>
							<div class="flex flex-col gap-1">
								<label class="text-sm font-medium" for="custom-region">Region</label>
								<input
									class="text-input-filled"
									id="custom-region"
									bind:value={form.customS3Config.region}
									placeholder="e.g. us-east-1"
								/>
							</div>
							<div class="flex flex-col gap-1">
								<label class="text-sm font-medium" for="custom-access-key">Access Key ID</label>
								<input
									name="custom-access-key"
									class="text-input-filled"
									bind:value={form.customS3Config.accessKeyID}
								/>
							</div>
							<div class="flex flex-col gap-1">
								<label class="text-sm font-medium" for="custom-secret-key">Secret Access Key</label>
								<SensitiveInput
									name="custom-secret-key"
									bind:value={form.customS3Config.secretAccessKey}
									hideReveal
									placeholder={existingCredentials?.customS3Config
										? '••••••••••••••••••••••••••••••••••••••••'
										: ''}
								/>
							</div>
						</div>
					{/if}
				</div>
			{/if}

			<!-- Test Result -->
			{#if testResult}
				<div
					class={`flex items-start gap-3 rounded-md p-4 ${testResult.success ? 'bg-green-50 dark:bg-green-950/50' : 'bg-red-50 dark:bg-red-950/50'}`}
				>
					{#if testResult.success}
						<Success message={testResult.message} />
					{:else}
						<AlertTriangle class="size-5 text-red-600 dark:text-red-400" />
						<div class="text-sm text-red-700 dark:text-red-300">
							{testResult.message}
						</div>
					{/if}
				</div>
			{/if}

			<!-- Error Display -->
			{#if error}
				<div class="flex items-start gap-3 rounded-md bg-red-50 p-4 dark:bg-red-950/50">
					<AlertTriangle class="size-5 text-red-600 dark:text-red-400" />
					<div class="text-sm text-red-700 dark:text-red-300">
						{error}
					</div>
				</div>
			{/if}

			<!-- Actions -->
			<div class="flex justify-between pt-6">
				<button
					type="button"
					class="button-secondary"
					onclick={handleTest}
					disabled={testing || saving}
				>
					{#if testing}
						<LoaderCircle class="size-4 animate-spin" />
						Testing...
					{:else}
						Test Connection
					{/if}
				</button>

				<div class="flex gap-3">
					<button type="button" class="button" onclick={onCancel} disabled={saving || testing}>
						Cancel
					</button>
					<button type="submit" class="button-primary" disabled={saving || testing}>
						{#if saving}
							<LoaderCircle class="size-4 animate-spin" />
							Saving...
						{:else}
							Save Credentials
						{/if}
					</button>
				</div>
			</div>
		</form>
	</div>
{/if}
