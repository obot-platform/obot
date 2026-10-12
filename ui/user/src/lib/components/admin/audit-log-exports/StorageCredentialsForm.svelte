<script lang="ts">
	import Confirm from '$lib/components/Confirm.svelte';
	import Select from '$lib/components/Select.svelte';
	import SensitiveInput from '$lib/components/SensitiveInput.svelte';
	import Success from '$lib/components/Success.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService } from '$lib/services';
	import type { StorageCredentials } from '$lib/services/admin/types';
	import { TriangleAlert, Trash } from '@lucide/svelte';
	import { onMount } from 'svelte';
	import { twMerge } from 'tailwind-merge';

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
	let deleting = $state(false);
	let showDeleteConfirm = $state(false);
	let error = $state('');
	let testResult = $state<{ success: boolean; message: string } | null>(null);
	let existingCredentials = $state<StorageCredentials | null>(null);

	// Load existing storage credentials on mount
	onMount(async () => {
		loadStorageProvider();
	});

	async function loadStorageProvider() {
		loading = true;
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
	}

	async function handleSubmit() {
		try {
			saving = true;
			error = '';

			// Validate required fields based on provider and auth method
			if (!useWorkloadIdentity) {
				if (form.provider === 's3') {
					if (!form.s3Config?.region) {
						throw new Error(
							m.audit_usage_exports_field_required({
								field: m.audit_usage_exports_region(),
								provider: 'S3'
							})
						);
					}
					if (!form.s3Config?.accessKeyID) {
						throw new Error(
							m.audit_usage_exports_field_required({
								field: m.audit_usage_exports_access_key_id(),
								provider: 'S3'
							})
						);
					}
					if (!form.s3Config?.secretAccessKey && !existingCredentials?.s3Config) {
						throw new Error(
							m.audit_usage_exports_field_required({
								field: m.audit_usage_exports_secret_access_key(),
								provider: 'S3'
							})
						);
					}
				} else if (form.provider === 'gcs') {
					if (!form.gcsConfig?.serviceAccountJSON && !existingCredentials?.gcsConfig) {
						throw new Error(
							m.audit_usage_exports_field_required({
								field: m.audit_usage_exports_service_account_json(),
								provider: 'GCS'
							})
						);
					}
				} else if (form.provider === 'azure') {
					if (!form.azureConfig?.storageAccount) {
						throw new Error(
							m.audit_usage_exports_field_required({
								field: m.audit_usage_exports_storage_account(),
								provider: 'Azure'
							})
						);
					}
					if (!form.azureConfig?.clientID) {
						throw new Error(
							m.audit_usage_exports_field_required({
								field: m.audit_usage_exports_client_id(),
								provider: 'Azure'
							})
						);
					}
					if (!form.azureConfig?.tenantID) {
						throw new Error(
							m.audit_usage_exports_field_required({
								field: m.audit_usage_exports_tenant_id(),
								provider: 'Azure'
							})
						);
					}
					if (!form.azureConfig?.clientSecret && !existingCredentials?.azureConfig) {
						throw new Error(
							m.audit_usage_exports_field_required({
								field: m.audit_usage_exports_client_secret(),
								provider: 'Azure'
							})
						);
					}
				} else if (form.provider === 'custom') {
					if (!form.customS3Config?.endpoint) {
						throw new Error(
							m.audit_usage_exports_field_required({
								field: m.audit_usage_exports_endpoint(),
								provider: m.audit_usage_exports_custom_s3_short()
							})
						);
					}
					if (!form.customS3Config?.region) {
						throw new Error(
							m.audit_usage_exports_field_required({
								field: m.audit_usage_exports_region(),
								provider: m.audit_usage_exports_custom_s3_short()
							})
						);
					}
					if (!form.customS3Config?.accessKeyID) {
						throw new Error(
							m.audit_usage_exports_field_required({
								field: m.audit_usage_exports_access_key_id(),
								provider: m.audit_usage_exports_custom_s3_short()
							})
						);
					}
					if (!form.customS3Config?.secretAccessKey && !existingCredentials?.customS3Config) {
						throw new Error(
							m.audit_usage_exports_field_required({
								field: m.audit_usage_exports_secret_access_key(),
								provider: m.audit_usage_exports_custom_s3_short()
							})
						);
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
			error = err instanceof Error ? err.message : m.audit_usage_exports_configure_failed();
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
				message: err instanceof Error ? err.message : m.audit_usage_exports_test_failed()
			};
		} finally {
			testing = false;
		}
	}

	function confirmDeleteCredentials() {
		showDeleteConfirm = true;
	}

	async function handleDeleteCredentials() {
		try {
			deleting = true;
			error = '';
			showDeleteConfirm = false;

			await AdminService.deleteStorageCredentials();

			existingCredentials = null;
			testResult = {
				success: true,
				message: m.audit_usage_exports_deleted()
			};

			// Reset form to default state
			form = {
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
			};
			useWorkloadIdentity = false;
		} catch (err) {
			error = err instanceof Error ? err.message : m.audit_usage_exports_delete_failed();
		} finally {
			deleting = false;
		}
	}
</script>

{#if loading}
	<div class="dark:bg-base-300 bg-base-100 rounded-md p-6 shadow-sm">
		<div class="flex items-center justify-center py-8">
			<Loading class="size-6" />
			<span class="ml-2 text-sm text-gray-600">{m.audit_usage_exports_loading()}</span>
		</div>
	</div>
{:else}
	<div class="paper">
		<form
			class="gap-8"
			onsubmit={(e) => {
				e.preventDefault();
				handleSubmit();
			}}
		>
			{#if existingCredentials}
				<div class="mb-6 flex items-start gap-3 rounded-md border border-warning bg-warning/10 p-4">
					<TriangleAlert class="size-5 shrink-0 text-warning" />
					<div class="flex-1 text-sm">
						<p class="font-medium">{m.audit_usage_exports_already_configured()}</p>
						<p class="mt-1 opacity-80">
							{m.audit_usage_exports_already_configured_prefix()}<span class="uppercase"
								>{existingCredentials.provider}</span
							>{m.audit_usage_exports_already_configured_suffix()}
						</p>
					</div>
				</div>
			{/if}

			<div class={twMerge('flex flex-col gap-8')}>
				<!-- Provider Selection -->
				<div class="space-y-4">
					<h3 class="text-lg font-semibold">{m.audit_usage_exports_provider_heading()}</h3>
					<div class="flex flex-col gap-1">
						<label class="text-sm font-medium" for="storage-provider">{m.core_col_provider()}</label
						>
						<div class={[!!existingCredentials && 'pointer-events-none opacity-50']}>
							<Select
								class="text-input-filled bg-base-200 dark:bg-base-100"
								classes={{ root: 'w-full' }}
								options={[
									{ id: 's3', label: m.audit_usage_exports_amazon_s3() },
									{ id: 'gcs', label: m.audit_usage_exports_google_cloud_storage() },
									{ id: 'azure', label: m.audit_usage_exports_azure_blob_storage() },
									{ id: 'custom', label: m.audit_usage_exports_custom_s3() }
								]}
								selected={form.provider}
								disabled={!!existingCredentials}
								onSelect={(value) => {
									form.provider = value.id;
									testResult = null; // Clear test result when provider changes

									// Clear other provider configs and initialize selected one
									if (value.id === 's3') {
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
									} else if (value.id === 'gcs') {
										form.s3Config = undefined;
										form.azureConfig = undefined;
										form.customS3Config = undefined;
										form.gcsConfig = {
											serviceAccountJSON: existingCredentials?.gcsConfig?.serviceAccountJSON || ''
										};
									} else if (value.id === 'azure') {
										form.s3Config = undefined;
										form.gcsConfig = undefined;
										form.customS3Config = undefined;
										form.azureConfig = {
											storageAccount: existingCredentials?.azureConfig?.storageAccount || '',
											clientID: existingCredentials?.azureConfig?.clientID || '',
											tenantID: existingCredentials?.azureConfig?.tenantID || '',
											clientSecret: existingCredentials?.azureConfig?.clientSecret || ''
										};
									} else if (value.id === 'custom') {
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

				{#if form.provider === 's3' && form.s3Config}
					<div class="flex flex-col gap-1">
						<label class="text-sm font-medium" for="region">{m.audit_usage_exports_region()}</label>
						<input
							class="text-input-filled"
							id="region"
							bind:value={form.s3Config.region}
							placeholder={m.audit_usage_exports_region_placeholder()}
						/>
					</div>
				{/if}
				<div class="divider my-0"></div>

				{#if form.provider === 'azure' && form.azureConfig}
					<div class="space-y-4">
						<div class="grid grid-cols-1 gap-6 md:grid-cols-2">
							<div class="flex flex-col gap-1">
								<label class="text-sm font-medium" for="storage-account"
									>{m.audit_usage_exports_storage_account()}</label
								>
								<input
									class="text-input-filled"
									id="storage-account"
									bind:value={form.azureConfig.storageAccount}
									placeholder="my-storage-account"
								/>
							</div>
						</div>
					</div>
				{/if}

				<!-- Authentication Method -->
				{#if form.provider !== 'custom'}
					<div class="space-y-4">
						<h3 class="text-lg font-semibold">{m.audit_usage_exports_auth_method()}</h3>
						<div class="flex flex-col gap-4">
							<div class="flex items-center justify-between">
								<div class="flex flex-col gap-1">
									<label class="text-sm font-medium" for="auth-method"
										>{m.audit_usage_exports_use_obot_credential()}</label
									>
								</div>
								<Toggle
									checked={useWorkloadIdentity}
									onChange={(checked) => (useWorkloadIdentity = checked)}
									label={useWorkloadIdentity
										? m.audit_usage_exports_use_workload_identity()
										: m.audit_usage_exports_configure_keys_manually()}
								/>
							</div>
							{#if useWorkloadIdentity}
								<div class="rounded-md bg-blue-50 p-4 dark:bg-blue-950/50">
									<p class="text-sm text-blue-700 dark:text-blue-300">
										{m.audit_usage_exports_using_workload_identity()}
									</p>
								</div>
							{/if}
						</div>
					</div>
				{/if}

				<!-- Credentials -->
				{#if !useWorkloadIdentity}
					<div class="space-y-4">
						<h3 class="text-lg font-semibold">{m.audit_usage_exports_credentials()}</h3>

						{#if form.provider === 's3' && form.s3Config}
							<div class="space-y-4">
								<div class="flex flex-col gap-1">
									<label class="text-sm font-medium" for="access-key"
										>{m.audit_usage_exports_access_key_id()}</label
									>
									<input
										name="access-key"
										class="text-input-filled"
										bind:value={form.s3Config.accessKeyID}
									/>
								</div>
								<div class="flex flex-col gap-1">
									<label class="text-sm font-medium" for="secret-key"
										>{m.audit_usage_exports_secret_access_key()}</label
									>
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
								<label class="text-sm font-medium" for="service-account"
									>{m.audit_usage_exports_service_account_json()}</label
								>
								<SensitiveInput
									name="service-account-json"
									bind:value={form.gcsConfig.serviceAccountJSON}
									placeholder={existingCredentials?.gcsConfig
										? '••••••••••••••••••••••••••••••••••••••••'
										: ''}
									textarea
									growable
									hideReveal
								/>
								<p class="text-muted-content text-xs">
									{m.audit_usage_exports_service_account_help()}
								</p>
							</div>
						{:else if form.provider === 'azure' && form.azureConfig}
							<div class="space-y-4">
								<div class="flex flex-col gap-1">
									<label class="text-sm font-medium" for="azure-client-id"
										>{m.audit_usage_exports_client_id()}</label
									>
									<input
										name="azure-client-id"
										class="text-input-filled"
										bind:value={form.azureConfig.clientID}
									/>
								</div>
								<div class="flex flex-col gap-1">
									<label class="text-sm font-medium" for="azure-tenant-id"
										>{m.audit_usage_exports_tenant_id()}</label
									>
									<input
										name="azure-tenant-id"
										class="text-input-filled"
										bind:value={form.azureConfig.tenantID}
									/>
								</div>
								<div class="flex flex-col gap-1">
									<label class="text-sm font-medium" for="azure-client-secret"
										>{m.audit_usage_exports_client_secret()}</label
									>
									<SensitiveInput
										name="azure-client-secret"
										bind:value={form.azureConfig.clientSecret}
										placeholder={existingCredentials?.azureConfig
											? '••••••••••••••••••••••••••••••••••••••••'
											: ''}
										hideReveal
									/>
								</div>
							</div>
						{:else if form.provider === 'custom' && form.customS3Config}
							<div class="space-y-4">
								<div class="flex flex-col gap-1">
									<label class="text-sm font-medium" for="custom-endpoint"
										>{m.audit_usage_exports_endpoint()}</label
									>
									<input
										class="text-input-filled"
										id="custom-endpoint"
										bind:value={form.customS3Config.endpoint}
										placeholder="https://s3.example.com"
									/>
								</div>
								<div class="flex flex-col gap-1">
									<label class="text-sm font-medium" for="custom-region"
										>{m.audit_usage_exports_region()}</label
									>
									<input
										class="text-input-filled"
										id="custom-region"
										bind:value={form.customS3Config.region}
										placeholder={m.audit_usage_exports_region_placeholder()}
									/>
								</div>
								<div class="flex flex-col gap-1">
									<label class="text-sm font-medium" for="custom-access-key"
										>{m.audit_usage_exports_access_key_id()}</label
									>
									<input
										name="custom-access-key"
										class="text-input-filled"
										bind:value={form.customS3Config.accessKeyID}
									/>
								</div>
								<div class="flex flex-col gap-1">
									<label class="text-sm font-medium" for="custom-secret-key"
										>{m.audit_usage_exports_secret_access_key()}</label
									>
									<SensitiveInput
										name="custom-secret-key"
										bind:value={form.customS3Config.secretAccessKey}
										placeholder={existingCredentials?.customS3Config
											? '••••••••••••••••••••••••••••••••••••••••'
											: ''}
										hideReveal
									/>
								</div>
							</div>
						{/if}
					</div>
				{/if}

				<!-- Test Result -->
				{#if testResult}
					<div
						class={`flex items-start gap-3 rounded-md p-4 ${testResult.success ? 'bg-success/10 text-success' : 'bg-error/10 text-error'}`}
					>
						{#if testResult.success}
							<Success message={testResult.message} />
						{:else}
							<TriangleAlert class="size-5 text-error" />
							<div class="text-sm text-error">
								{testResult.message}
							</div>
						{/if}
					</div>
				{/if}

				<!-- Error Display -->
				{#if error}
					<div class="flex items-start gap-3 rounded-md bg-error/10 p-4">
						<TriangleAlert class="size-5 text-error" />
						<div class="text-sm text-error">
							{error}
						</div>
					</div>
				{/if}
			</div>

			<!-- Actions -->
			<div class="flex justify-between gap-4 pt-6">
				{#if form.provider !== 'custom'}
					<button
						type="button"
						class="btn btn-secondary"
						onclick={handleTest}
						disabled={testing || saving}
					>
						{#if testing}
							<Loading class="size-4" />
							{m.audit_usage_exports_testing()}
						{:else}
							{m.audit_usage_exports_test_connection()}
						{/if}
					</button>
				{/if}

				{#if !!existingCredentials}
					<button
						type="button"
						class="btn btn-error"
						onclick={confirmDeleteCredentials}
						disabled={testing || saving || deleting}
					>
						{#if deleting}
							<Loading class="size-4" />
							{m.audit_usage_exports_deleting()}
						{:else}
							<Trash class="size-4" />
							{m.audit_usage_exports_delete_credentials()}
						{/if}
					</button>
				{/if}

				<div class="ml-auto flex gap-3">
					<button
						type="button"
						class="btn btn-secondary"
						onclick={onCancel}
						disabled={saving || testing}
					>
						{m.common_cancel()}
					</button>
					<button type="submit" class="btn btn-primary" disabled={saving || testing}>
						{#if saving}
							<Loading class="size-4" />
							{m.audit_usage_exports_saving()}
						{:else}
							{m.audit_usage_exports_save_credentials()}
						{/if}
					</button>
				</div>
			</div>
		</form>
	</div>
{/if}

<Confirm
	show={showDeleteConfirm}
	msg={existingCredentials?.provider
		? m.audit_usage_exports_delete_provider_msg({ provider: existingCredentials.provider })
		: m.audit_usage_exports_delete_these_msg()}
	onsuccess={handleDeleteCredentials}
	oncancel={() => (showDeleteConfirm = false)}
	loading={deleting}
/>
