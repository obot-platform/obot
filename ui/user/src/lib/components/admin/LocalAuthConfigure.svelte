<script lang="ts">
	import { resolve } from '$app/paths';
	import ResponsiveDialog, {
		type ResponsiveDialogAnimate
	} from '$lib/components/ResponsiveDialog.svelte';
	import LocalAuthInitialUserForm from '$lib/components/admin/LocalAuthInitialUserForm.svelte';
	import { MultiValueInput } from '$lib/components/ui/multi-value-input';
	import { m } from '$lib/i18n';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService, type AuthProvider } from '$lib/services';
	import { darkMode } from '$lib/stores';
	import { CircleAlert } from '@lucide/svelte';
	import type { Snippet } from 'svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		provider?: AuthProvider;
		values?: Record<string, string>;
		readonly?: boolean;
		// Save the email-domains config. Returns an error message, or undefined on success.
		onConfigure: (form: Record<string, string>) => Promise<string | undefined>;
		// Called after the modal closes, with the number of local users that currently exist.
		onClose?: (userCount: number) => void;
		animate?: ResponsiveDialogAnimate;
		// First-time onboarding: skip domain collection, auto-configure, and collect a single
		// initial user. Later user changes happen on the Users page.
		required?: boolean;
		switchingProvider?: boolean;
		additionalActions?: Snippet;
	}

	const {
		provider,
		values,
		readonly,
		onConfigure,
		onClose,
		animate,
		required,
		switchingProvider,
		additionalActions
	}: Props = $props();

	const DOMAINS_KEY = 'OBOT_AUTH_PROVIDER_EMAIL_DOMAINS';

	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let createInitialUserDialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let domains = $state('');
	let configuring = $state(false);
	let configError = $state<string>();
	let configured = $state(false);
	let initialUserDialogOpen = $state(false);
	let initialUserCount = $state(0);

	function openBootstrapSetup() {
		initialUserCount = 0;
		initialUserDialogOpen = true;
		createInitialUserDialog?.open();
	}

	export function open() {
		if (required) {
			openBootstrapSetup();
			return;
		}

		domains = values?.[DOMAINS_KEY] ?? '*';
		configError = undefined;
		configured = !!(provider?.configured || provider?.staged);
		dialog?.open();
	}

	export function close() {
		dialog?.close();
		initialUserDialogOpen = false;
		createInitialUserDialog?.close();
	}

	async function handleDialogClose() {
		try {
			onClose?.((await AdminService.listLocalAuthUsers()).length);
		} catch {
			onClose?.(0);
		}
	}

	async function handleSubmit(e?: SubmitEvent) {
		e?.preventDefault();
		if (readonly) return;

		if (!domains.trim()) {
			configError = m.identity_access_auth_providers_local_auth_domains_required();
			return;
		}

		configuring = true;
		configError = undefined;
		try {
			const err = await onConfigure({ [DOMAINS_KEY]: domains });
			if (err) {
				configError = err;
				return;
			}
			configured = true;
		} finally {
			configuring = false;
		}
	}
</script>

<ResponsiveDialog
	bind:this={dialog}
	class="md:w-xl"
	onClose={handleDialogClose}
	{animate}
	disableClickOutside={required}
	hideClose={required}
>
	{#snippet titleContent()}
		{@render setupTitle()}
	{/snippet}

	<form class="flex flex-col gap-4 p-4 md:p-0" onsubmit={handleSubmit}>
		{#if configError}
			<div class="notification-error flex items-center gap-2">
				<CircleAlert class="text-error size-5 shrink-0" />
				<p class="text-sm font-light">{configError}</p>
			</div>
		{/if}

		<div class="notification-info text-xs">
			{m.identity_access_auth_providers_local_auth_provider_note()}
		</div>

		<div class="notification-info text-xs">
			{m.identity_access_auth_providers_local_auth_add_user_prefix()}<a
				class="link link-primary"
				href={resolve('/identity-access?view=users')}
				>{m.nav_identity_access()} → {m.identity_access_users_tab()}</a
			>{m.identity_access_auth_providers_local_auth_add_user_suffix()}
		</div>

		<div class="flex flex-col gap-1">
			<label for="local-auth-domains"
				>{m.identity_access_auth_providers_local_auth_allowed_domains()}</label
			>
			<span class="text-gray text-xs">
				{m.identity_access_auth_providers_local_auth_allowed_domains_hint()}
			</span>
			<MultiValueInput
				bind:value={domains}
				id="local-auth-domains"
				labels={{ '*': m.identity_access_auth_providers_local_auth_all_domains() }}
				class="text-input-filled"
				placeholder={m.identity_access_auth_providers_local_auth_enter_to_insert()}
				disabled={readonly}
			/>
		</div>

		<div class="flex justify-between">
			<div>
				{#if additionalActions}
					{@render additionalActions?.()}
				{/if}
			</div>
			{#if !readonly && (!configured || switchingProvider)}
				<button class="btn btn-primary" type="submit" disabled={configuring}>
					{#if configuring}
						<Loading class="size-4" />
					{:else}
						{m.core_continue()}
					{/if}
				</button>
			{:else if !readonly}
				<button class="btn btn-primary" type="submit" disabled={configuring}>
					{#if configuring}
						<Loading class="size-4" />
					{:else}
						{m.core_save()}
					{/if}
				</button>
			{/if}
		</div>
	</form>
</ResponsiveDialog>

<ResponsiveDialog
	bind:this={createInitialUserDialog}
	class="w-xl"
	onClose={() => {
		initialUserDialogOpen = false;
		onClose?.(initialUserCount);
	}}
	{animate}
	disableClickOutside
	hideClose
>
	{#if initialUserDialogOpen}
		<LocalAuthInitialUserForm
			{provider}
			{readonly}
			{onConfigure}
			{additionalActions}
			onCreated={(count) => {
				initialUserCount = count;
				close();
			}}
		/>
	{/if}
</ResponsiveDialog>

{#snippet setupTitle()}
	<div class="flex items-center gap-2">
		{#if darkMode.isDark}
			{@const url = provider?.iconDark ?? provider?.icon}
			<img
				src={url}
				alt={provider?.name}
				class={twMerge('size-9 rounded-md p-1', !provider?.iconDark && 'bg-base-300')}
			/>
		{:else}
			<img src={provider?.icon} alt={provider?.name} class="bg-base-200 size-9 rounded-md p-1" />
		{/if}
		{m.identity_access_auth_providers_local_auth_set_up({ name: provider?.name ?? '' })}
	</div>
{/snippet}
