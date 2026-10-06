<script lang="ts">
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import ProviderDeconfigureConfirm from '$lib/components/admin/ProviderDeconfigureConfirm.svelte';
	import { m } from '$lib/i18n';
	import { reloadPage } from '$lib/navigation';
	import {
		AdminService,
		type AuthProvider,
		type LicenseEntitlementViolation,
		type ModelProvider
	} from '$lib/services';
	import { version, license, darkMode } from '$lib/stores';
	import { adminConfigStore } from '$lib/stores/adminConfig.svelte';
	import LicenseProviderDialog from './LicenseProviderDialog.svelte';
	import { KeyRound, Mail } from '@lucide/svelte';
	import { slide } from 'svelte/transition';

	interface Props {
		warnUserLimit?: boolean;
	}

	const { warnUserLimit }: Props = $props();

	let licenseViolationDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let confirmDowngradeDialog = $state<ReturnType<typeof ProviderDeconfigureConfirm>>();
	let providersToDeconfigure = $state<(AuthProvider | ModelProvider)[]>([]);

	let licenseRequiredProvider = $state<AuthProvider>();

	let downgrading = $state(false);
	let error = $state('');
	let violations = $derived(
		(version.current.licenseEntitlementViolations || []).reduce(
			(arr, violation) => {
				if (violation.type === 'authProvider') {
					arr.authProvider = true;
				} else if (violation.type === 'modelProvider') {
					arr.modelProvider = true;
				} else if (violation.type === 'userLimit') {
					arr.userLimit = true;
				}
				return arr;
			},
			{
				authProvider: false,
				modelProvider: false,
				userLimit: false
			}
		)
	);

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

			reloadPage();
		} catch (err) {
			error = err instanceof Error ? err.message : m.platform_license_unknown_error();
		} finally {
			confirmDowngradeDialog?.close();
			downgrading = false;
		}
	}
</script>

<ResponsiveDialog
	bind:this={licenseViolationDialog}
	title={warnUserLimit || violations.userLimit
		? warnUserLimit
			? m.platform_license_notice_nearing_user_limit()
			: m.platform_license_notice_user_limit_reached()
		: m.platform_license_notice_missing_or_invalid()}
	class="md:max-w-md"
>
	<div class="md:p-0 p-4">
		<div class="flex flex-col gap-4">
			<p class="font-light text-center">
				{#if warnUserLimit || violations.userLimit}
					{m.platform_license_notice_unlock_prefix()}
					<a href="mailto:info@obot.ai" class="text-link">info@obot.ai</a
					>{m.platform_license_notice_unlock_suffix()}
				{:else}
					{violations.authProvider
						? m.platform_license_notice_reenable_register_prefix()
						: m.platform_license_notice_reenable_prefix()}
					<a href="mailto:info@obot.ai" class="text-link">info@obot.ai</a
					>{m.platform_license_notice_reenable_suffix()}
				{/if}
			</p>
			{#if violations.authProvider}
				<button
					class="btn btn-primary"
					onclick={() => {
						const violationId = version.current.licenseEntitlementViolations?.find(
							(v) => v.type === 'authProvider'
						)?.name;
						licenseRequiredProvider = violationId
							? $adminConfigStore.authProviders.find((p) => p.id === violationId)
							: undefined;
					}}
				>
					<KeyRound class="size-4" />
					{m.platform_license_notice_register_your_email()}
				</button>
			{/if}
			<a href="mailto:info@obot.ai" class="btn btn-secondary">
				<Mail class="size-4" />
				{m.platform_license_notice_contact_support()}
			</a>
		</div>
		{#if !warnUserLimit && (violations.authProvider || violations.modelProvider)}
			<div class="divider">{m.platform_license_notice_or()}</div>
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
								<p class="font-semibold">
									{m.platform_license_notice_deconfigure_named({ name: provider.name })}
								</p>
								<p class="text-xs text-muted-content">
									{#if violation.type === 'authProvider'}
										{m.platform_license_notice_deconfigure_auth_note({ name: provider.name })}
									{:else}
										{m.platform_license_notice_deconfigure_model_note({ name: provider.name })}
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
					{m.platform_license_notice_downgrade()}
				</button>
			</div>
		{/if}
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
	title={m.platform_license_notice_confirm_downgrade()}
	confirmButtonText={m.platform_license_notice_downgrade()}
/>

<LicenseProviderDialog
	bind:provider={licenseRequiredProvider}
	licenseKey={license.current.licenseKey}
	endpoint={AdminService.createCommunityLicense}
	onSubmit={() => reloadPage()}
	allowSignup
	signUpMessage={m.platform_license_notice_signup_message()}
/>
