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

	// Track which fields have existing masked values
	let maskedFields = $state({
		s3AccessKeyID: false,
		s3SecretAccessKey: false,
		s3SessionToken: false,
		gcsServiceAccountJSON: false,
		azureClientID: false,
		azureTenantID: false,
		azureClientSecret: false,
		customS3AccessKeyID: false,
		customS3SecretAccessKey: false
	});

	let saving = $state(false);
	let testing = $state(false);
	let loading = $state(true);
	let error = $state('');
	let testResult = $state<{ success: boolean; message: string } | null>(null);

	// Load existing storage credentials on mount
	onMount(async () => {
		try {
			const existingCredentials = await AdminService.getStorageCredentials();
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

					// Set masked values if credentials exist but values are empty (backend doesn't return actual values)
					if (!existingCredentials.useWorkloadIdentity && existingCredentials.provider === 's3') {
						if (!form.s3Config.accessKeyID) {
							form.s3Config.accessKeyID = '••••••••••••••••';
							maskedFields.s3AccessKeyID = true;
						}
						if (!form.s3Config.secretAccessKey) {
							form.s3Config.secretAccessKey = '••••••••••••••••••••••••••••••••••••••••';
							maskedFields.s3SecretAccessKey = true;
						}
					}
					form.gcsConfig = undefined;
					form.azureConfig = undefined;
					form.customS3Config = undefined;
					useWorkloadIdentity = existingCredentials.useWorkloadIdentity;
				} else if (existingCredentials.gcsConfig) {
					form.gcsConfig = {
						serviceAccountJSON: existingCredentials.gcsConfig.serviceAccountJSON || ''
					};

					// Set masked values if credentials exist but values are empty
					if (!existingCredentials.useWorkloadIdentity && existingCredentials.provider === 'gcs') {
						if (!form.gcsConfig.serviceAccountJSON) {
							form.gcsConfig.serviceAccountJSON =
								'••••••••••••••••••••••••••••••••••••••••••••••••••••••••';
							maskedFields.gcsServiceAccountJSON = true;
						}
					}

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

					if (
						!existingCredentials.useWorkloadIdentity &&
						existingCredentials.provider === 'azure'
					) {
						if (!form.azureConfig.clientID) {
							form.azureConfig.clientID = '••••••••••••••••';
							maskedFields.azureClientID = true;
						}
						if (!form.azureConfig?.tenantID) {
							form.azureConfig.tenantID = '••••••••••••••••';
							maskedFields.azureTenantID = true;
						}
						if (!form.azureConfig?.clientSecret) {
							form.azureConfig.clientSecret = '••••••••••••••••••••••••••••••••••••••••';
							maskedFields.azureClientSecret = true;
						}
					}
					useWorkloadIdentity = existingCredentials.useWorkloadIdentity;
				} else if (existingCredentials.customS3Config) {
					form.customS3Config = {
						endpoint: existingCredentials.customS3Config.endpoint || '',
						region: existingCredentials.customS3Config.region || '',
						accessKeyID: '••••••••••••••••',
						secretAccessKey: '••••••••••••••••••••••••••••••••••••••••'
					};
					maskedFields.customS3AccessKeyID = true;
					maskedFields.customS3SecretAccessKey = true;
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

	// Functions to clear masked values when user starts typing
	function handleS3AccessKeyIDFocus() {
		if (maskedFields.s3AccessKeyID && form.s3Config) {
			form.s3Config.accessKeyID = '';
			maskedFields.s3AccessKeyID = false;
		}
	}

	function handleS3SecretAccessKeyFocus() {
		if (maskedFields.s3SecretAccessKey && form.s3Config) {
			form.s3Config.secretAccessKey = '';
			maskedFields.s3SecretAccessKey = false;
		}
	}

	function handleGcsServiceAccountJSONFocus() {
		if (maskedFields.gcsServiceAccountJSON && form.gcsConfig) {
			form.gcsConfig.serviceAccountJSON = '';
			maskedFields.gcsServiceAccountJSON = false;
		}
	}

	function handleAzureClientIDFocus() {
		if (maskedFields.azureClientID && form.azureConfig) {
			form.azureConfig.clientID = '';
			maskedFields.azureClientID = false;
		}
	}

	function handleAzureTenantIDFocus() {
		if (maskedFields.azureTenantID && form.azureConfig) {
			form.azureConfig.tenantID = '';
			maskedFields.azureTenantID = false;
		}
	}

	function handleAzureClientSecretFocus() {
		if (maskedFields.azureClientSecret && form.azureConfig) {
			form.azureConfig.clientSecret = '';
			maskedFields.azureClientSecret = false;
		}
	}

	function handleCustomS3AccessKeyIDFocus() {
		if (maskedFields.customS3AccessKeyID && form.customS3Config) {
			form.customS3Config.accessKeyID = '';
			maskedFields.customS3AccessKeyID = false;
		}
	}

	function handleCustomS3SecretAccessKeyFocus() {
		if (maskedFields.customS3SecretAccessKey && form.customS3Config) {
			form.customS3Config.secretAccessKey = '';
			maskedFields.customS3SecretAccessKey = false;
		}
	}

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
					// Only validate credentials if they're not masked (user is providing new ones)
					if (!maskedFields.s3AccessKeyID && !form.s3Config?.accessKeyID) {
						throw new Error('Access Key ID is required for S3');
					}
					if (!maskedFields.s3SecretAccessKey && !form.s3Config?.secretAccessKey) {
						throw new Error('Secret Access Key is required for S3');
					}
				} else if (form.provider === 'gcs') {
					// Only validate if not masked
					if (!maskedFields.gcsServiceAccountJSON && !form.gcsConfig?.serviceAccountJSON) {
						throw new Error('Service Account JSON is required for GCS');
					}
				} else if (form.provider === 'azure') {
					// Only validate if not masked
					if (!maskedFields.azureClientID && !form.azureConfig?.clientID) {
						throw new Error('Client ID is required for Azure');
					}
					if (!maskedFields.azureTenantID && !form.azureConfig?.tenantID) {
						throw new Error('Tenant ID is required for Azure');
					}
					if (!maskedFields.azureClientSecret && !form.azureConfig?.clientSecret) {
						throw new Error('Client Secret is required for Azure');
					}
				} else if (form.provider === 'custom') {
					if (!form.customS3Config?.endpoint) {
						throw new Error('Endpoint is required for Custom S3');
					}
					if (!form.customS3Config?.region) {
						throw new Error('Region is required for Custom S3');
					}
					// Only validate credentials if not masked
					if (!maskedFields.customS3AccessKeyID && !form.customS3Config?.accessKeyID) {
						throw new Error('Access Key ID is required for Custom S3');
					}
					if (!maskedFields.customS3SecretAccessKey && !form.customS3Config?.secretAccessKey) {
						throw new Error('Secret Access Key is required for Custom S3');
					}
				}
			}

			// Prepare the request - clean masked fields
			const request = {
				...form,
				useWorkloadIdentity
			};

			// Clean masked fields before sending
			if (request.s3Config) {
				request.s3Config = {
					...request.s3Config,
					accessKeyID: maskedFields.s3AccessKeyID ? '' : request.s3Config.accessKeyID,
					secretAccessKey: maskedFields.s3SecretAccessKey ? '' : request.s3Config.secretAccessKey
				};
			}

			if (request.gcsConfig) {
				request.gcsConfig = {
					...request.gcsConfig,
					serviceAccountJSON: maskedFields.gcsServiceAccountJSON
						? ''
						: request.gcsConfig.serviceAccountJSON
				};
			}

			if (request.azureConfig) {
				request.azureConfig = {
					...request.azureConfig,
					clientID: maskedFields.azureClientID ? '' : request.azureConfig.clientID,
					tenantID: maskedFields.azureTenantID ? '' : request.azureConfig.tenantID,
					clientSecret: maskedFields.azureClientSecret ? '' : request.azureConfig.clientSecret
				};
			}

			if (request.customS3Config) {
				request.customS3Config = {
					...request.customS3Config,
					accessKeyID: maskedFields.customS3AccessKeyID ? '' : request.customS3Config.accessKeyID,
					secretAccessKey: maskedFields.customS3SecretAccessKey
						? ''
						: request.customS3Config.secretAccessKey
				};
			}

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
					accessKeyID: maskedFields.s3AccessKeyID ? '' : request.s3Config.accessKeyID,
					secretAccessKey: maskedFields.s3SecretAccessKey ? '' : request.s3Config.secretAccessKey
				};
			}

			if (request.gcsConfig) {
				request.gcsConfig = {
					...request.gcsConfig,
					serviceAccountJSON: maskedFields.gcsServiceAccountJSON
						? ''
						: request.gcsConfig.serviceAccountJSON
				};
			}

			if (request.azureConfig) {
				request.azureConfig = {
					...request.azureConfig,
					clientID: maskedFields.azureClientID ? '' : request.azureConfig.clientID,
					tenantID: maskedFields.azureTenantID ? '' : request.azureConfig.tenantID,
					clientSecret: maskedFields.azureClientSecret ? '' : request.azureConfig.clientSecret
				};
			}

			if (request.customS3Config) {
				request.customS3Config = {
					...request.customS3Config,
					accessKeyID: maskedFields.customS3AccessKeyID ? '' : request.customS3Config.accessKeyID,
					secretAccessKey: maskedFields.customS3SecretAccessKey
						? ''
						: request.customS3Config.secretAccessKey
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

								// Clear other provider configs and initialize selected one
								if (value === 's3') {
									form.gcsConfig = undefined;
									form.azureConfig = undefined;
									form.customS3Config = undefined;
									if (!form.s3Config) {
										form.s3Config = {
											region: '',
											accessKeyID: '',
											secretAccessKey: '',
											sessionToken: ''
										};
									}
									useWorkloadIdentity = false;
								} else if (value === 'gcs') {
									form.s3Config = undefined;
									form.azureConfig = undefined;
									form.customS3Config = undefined;
									if (!form.gcsConfig) {
										form.gcsConfig = { serviceAccountJSON: '' };
									}
								} else if (value === 'azure') {
									form.s3Config = undefined;
									form.gcsConfig = undefined;
									form.customS3Config = undefined;
									if (!form.azureConfig) {
										form.azureConfig = {
											storageAccount: '',
											clientID: '',
											tenantID: '',
											clientSecret: ''
										};
									}
								} else if (value === 'custom') {
									form.s3Config = undefined;
									form.gcsConfig = undefined;
									form.azureConfig = undefined;
									if (!form.customS3Config) {
										form.customS3Config = {
											endpoint: '',
											region: '',
											accessKeyID: '',
											secretAccessKey: ''
										};
									}
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
								placeholder="us-east-1"
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
								<SensitiveInput
									name="access-key"
									bind:value={form.s3Config.accessKeyID}
									onfocus={handleS3AccessKeyIDFocus}
								/>
							</div>
							<div class="flex flex-col gap-1">
								<label class="text-sm font-medium" for="secret-key">Secret Access Key</label>
								<SensitiveInput
									name="secret-key"
									bind:value={form.s3Config.secretAccessKey}
									onfocus={handleS3SecretAccessKeyFocus}
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
								onfocus={handleGcsServiceAccountJSONFocus}
							/>
							<p class="text-xs text-gray-500">Complete JSON key file for the service account</p>
						</div>
					{:else if form.provider === 'azure' && form.azureConfig}
						<div class="space-y-4">
							<div class="flex flex-col gap-1">
								<label class="text-sm font-medium" for="azure-client-id">Client ID</label>
								<SensitiveInput
									name="azure-client-id"
									bind:value={form.azureConfig.clientID}
									onfocus={handleAzureClientIDFocus}
								/>
							</div>
							<div class="flex flex-col gap-1">
								<label class="text-sm font-medium" for="azure-tenant-id">Tenant ID</label>
								<SensitiveInput
									name="azure-tenant-id"
									bind:value={form.azureConfig.tenantID}
									onfocus={handleAzureTenantIDFocus}
								/>
							</div>
							<div class="flex flex-col gap-1">
								<label class="text-sm font-medium" for="azure-client-secret">Client Secret</label>
								<SensitiveInput
									name="azure-client-secret"
									bind:value={form.azureConfig.clientSecret}
									onfocus={handleAzureClientSecretFocus}
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
									placeholder="us-east-1"
								/>
							</div>
							<div class="flex flex-col gap-1">
								<label class="text-sm font-medium" for="custom-access-key">Access Key ID</label>
								<SensitiveInput
									name="custom-access-key"
									bind:value={form.customS3Config.accessKeyID}
									onfocus={handleCustomS3AccessKeyIDFocus}
								/>
							</div>
							<div class="flex flex-col gap-1">
								<label class="text-sm font-medium" for="custom-secret-key">Secret Access Key</label>
								<SensitiveInput
									name="custom-secret-key"
									bind:value={form.customS3Config.secretAccessKey}
									onfocus={handleCustomS3SecretAccessKeyFocus}
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
