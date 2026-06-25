<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import ProviderDeconfigureConfirm from '$lib/components/admin/ProviderDeconfigureConfirm.svelte';
	import {
		AdminService,
		type AuthProvider,
		type LicenseEntitlementViolation,
		type ModelProvider
	} from '$lib/services';
	import { version, darkMode } from '$lib/stores';
	import { adminConfigStore } from '$lib/stores/adminConfig.svelte';
	import { Mail } from '@lucide/svelte';
	import { slide } from 'svelte/transition';

	let licenseViolationDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let confirmDowngradeDialog = $state<ReturnType<typeof ProviderDeconfigureConfirm>>();
	let providersToDeconfigure = $state<(AuthProvider | ModelProvider)[]>([]);

	let downgrading = $state(false);
	let error = $state('');

	export function open() {
		licenseViolationDialog?.open();
	}
	export function close() {
		licenseViolationDialog?.close();
	}

	async function handleDowngrade() {
		if (!version.current.licenseEntitlementViolations) {
			console.error('No license entitlement violations found');
			return;
		}

		downgrading = true;
		try {
			// do model provider deconfigures first, then auth provider deconfigures
			const { modelProviderViolations, authProviderViolations } =
				version.current.licenseEntitlementViolations.reduce<{
					modelProviderViolations: LicenseEntitlementViolation[];
					authProviderViolations: LicenseEntitlementViolation[];
				}>(
					(acc, provider) => {
						if (provider.type === 'modelProvider') {
							acc.modelProviderViolations.push(provider);
						} else if (provider.type === 'authProvider') {
							acc.authProviderViolations.push(provider);
						}
						return acc;
					},
					{ modelProviderViolations: [], authProviderViolations: [] }
				);
			for (const modelProvider of modelProviderViolations) {
				await AdminService.deconfigureModelProvider(modelProvider.name);
			}
			for (const authProvider of authProviderViolations) {
				await AdminService.deconfigureAuthProvider(authProvider.name);
			}

			window.location.reload();
		} catch (err) {
			error = err instanceof Error ? err.message : 'An unknown error occurred.';
		} finally {
			confirmDowngradeDialog?.close();
			downgrading = false;
		}
	}
</script>

<ResponsiveDialog
	bind:this={licenseViolationDialog}
	title="Missing or Invalid License"
	class="md:max-w-md"
>
	<div class="md:p-0 p-4">
		<div class="flex flex-col gap-4">
			<p class="font-light">
				To re-enable full access to existing functionality, please contact support at
				<a href="mailto:licensing@obot.ai" class="text-link">licensing@obot.ai</a> to renew your license.
			</p>
			<a href="mailto:licensing@obot.ai" class="btn btn-primary">
				<Mail class="size-4" />
				Contact Support
			</a>
		</div>
		<div class="divider">OR</div>
		<div class="flex flex-col gap-4">
			{#each version.current.licenseEntitlementViolations as violation (violation.name)}
				{@const provider =
					violation.type === 'authProvider'
						? $adminConfigStore.authProviders.find((p) => p.id === violation.name)
						: $adminConfigStore.modelProviders.find((p) => p.id === violation.name)}
				{#if provider}
					<div class="flex justify-between gap-4">
						<div class="dark:bg-base-400 p-1 rounded-md shrink-0">
							{#if darkMode.isDark}
								{@const url = provider.iconDark || provider.icon}
								<img src={url} alt={provider.name} class="size-10 rounded-md p-1" />
							{:else}
								<img
									src={provider.icon}
									alt={provider.name}
									class="size-10 rounded-md p-1 dark:bg-base-400"
								/>
							{/if}
						</div>
						<div class="flex grow flex-col gap-0.5">
							<p class="font-semibold">Deconfigure {provider.name}</p>
							<p class="text-xs text-muted-content">
								{#if violation.type === 'authProvider'}
									Users logged in via {provider.name} will need to sign in via a different accessible
									provider.
								{:else}
									Deconfiguring this model provider will cause loss of access to the models provided
									by
									{provider.name}.
								{/if}
							</p>
						</div>
					</div>
				{/if}
			{/each}
			{#if error}
				<div
					role="alert"
					class="alert alert-error alert-soft"
					in:slide={{ duration: 150, axis: 'y' }}
				>
					{error}
				</div>
			{/if}
			<button
				class="btn btn-error btn-soft mt-2"
				onclick={() => {
					licenseViolationDialog?.close();
					providersToDeconfigure = (version.current.licenseEntitlementViolations || [])
						.map((violation) => {
							if (violation.type === 'authProvider') {
								return $adminConfigStore.authProviders.find((p) => p.id === violation.name);
							} else {
								return $adminConfigStore.modelProviders.find((p) => p.id === violation.name);
							}
						})
						.filter((p): p is AuthProvider | ModelProvider => p !== undefined);
					confirmDowngradeDialog?.open();
				}}
			>
				Downgrade
			</button>
		</div>
	</div>
</ResponsiveDialog>

<ProviderDeconfigureConfirm
	bind:this={confirmDowngradeDialog}
	onConfirm={handleDowngrade}
	onCancel={() => {
		confirmDowngradeDialog?.close();
		licenseViolationDialog?.open();
	}}
	loading={downgrading}
	providers={providersToDeconfigure}
	title="Confirm Downgrade"
	confirmButtonText="Downgrade"
/>
