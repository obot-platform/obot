<script lang="ts">
	import InfoTooltip from '$lib/components/InfoTooltip.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import { m } from '$lib/i18n';
	import type { ImagePullSecret, ImagePullSecretCapability } from '$lib/services';
	import CapabilityBanner from './CapabilityBanner.svelte';
	import ECRSetupGuide from './ECRSetupGuide.svelte';
	import FieldLabel from './FieldLabel.svelte';
	import {
		defaultECRAudience,
		ecrPolicyJSON,
		ecrTrustPolicyJSON,
		type ImagePullSecretFormState
	} from './types';
	import { ChevronDown, CircleCheck, Info, LoaderCircle, RefreshCw } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		form: ImagePullSecretFormState;
		showECRAdvanced: boolean;
		capability: ImagePullSecretCapability;
		currentSecret?: ImagePullSecret;
		selectedId?: string | null;
		mutationsDisabled?: boolean;
		saving?: boolean;
		refreshing?: boolean;
		refreshMessage?: string;
		requiredErrors?: Record<string, string>;
		hideSubmit?: boolean;
		showRefresh?: boolean;
		onSave: () => void;
		onRefresh: (secret: ImagePullSecret) => void;
	}

	let {
		form = $bindable(),
		showECRAdvanced = $bindable(),
		capability,
		currentSecret,
		selectedId,
		mutationsDisabled = false,
		saving = false,
		refreshing = false,
		refreshMessage = '',
		requiredErrors = {},
		hideSubmit = false,
		showRefresh = true,
		onSave,
		onRefresh
	}: Props = $props();

	let effectiveIssuerURL = $derived(form.issuerURL.trim() || capability.issuerURL || '');
	let effectiveSubject = $derived(currentSecret?.status?.subject || capability.subject || '');
	let effectiveAudience = $derived(
		form.audience.trim() || currentSecret?.manifest.ecr?.audience || capability.audience || ''
	);
	let issuerDiscoveryReason = $derived(
		form.type === 'ecr' && !effectiveIssuerURL && capability.available ? capability.reason : ''
	);
	let previewTrustPolicyJSON = $derived(
		ecrTrustPolicyJSON(form.roleARN, effectiveIssuerURL, effectiveSubject, effectiveAudience)
	);
	let previewECRPolicyJSON = $derived(ecrPolicyJSON());

	function inputClass(field: string) {
		return twMerge(
			'input-text-filled',
			requiredErrors[field] && 'border-red-500 focus:border-red-500 focus:ring-red-500'
		);
	}
</script>

{#if !capability.available}
	<CapabilityBanner reason={capability.reason} />
{/if}

{#if selectedId && !currentSecret}
	<div class="notification-info flex items-center gap-3">
		<Info class="size-5" />
		<div>{m.platform_settings_image_pull_secrets_not_found()}</div>
	</div>
{:else}
	<form
		class="flex flex-col gap-4"
		novalidate
		onsubmit={(e) => {
			e.preventDefault();
			if (!hideSubmit) onSave();
		}}
	>
		<div class="flex flex-col gap-4">
			<label class="flex flex-col gap-1">
				<FieldLabel
					label={m.platform_settings_image_pull_secrets_display_name()}
					help={m.platform_settings_image_pull_secrets_display_name_help()}
				/>
				<input
					class="input-text-filled"
					bind:value={form.displayName}
					disabled={mutationsDisabled}
					placeholder={form.type === 'ecr'
						? m.platform_settings_image_pull_secrets_display_name_ecr_placeholder()
						: m.platform_settings_image_pull_secrets_display_name_basic_placeholder()}
				/>
			</label>
		</div>

		{#if form.type === 'basic'}
			{@render basicFields()}
		{:else}
			{@render ecrFields()}
		{/if}

		{#if refreshMessage}
			<div
				class={twMerge(
					'flex items-center gap-3 rounded-md border p-3 text-sm',
					'border-green-500 bg-green-500/10 text-green-700 dark:text-green-300'
				)}
			>
				<CircleCheck class="size-5" />
				<span>{refreshMessage}</span>
			</div>
		{/if}

		{#if currentSecret}
			{@render enabledToggle()}
		{/if}

		{#if !hideSubmit || (showRefresh && currentSecret && form.type === 'ecr')}
			<div class="flex flex-wrap items-center justify-end gap-2">
				{#if showRefresh && currentSecret && form.type === 'ecr'}
					<button
						type="button"
						class="btn btn-secondary flex items-center gap-1 text-sm"
						disabled={mutationsDisabled || refreshing}
						onclick={() => onRefresh(currentSecret)}
					>
						<RefreshCw class={twMerge('size-4', refreshing && 'animate-spin')} />
						{m.platform_refresh_now()}
					</button>
				{/if}
				{#if !hideSubmit}
					<button
						type="submit"
						class="btn btn-primary flex items-center gap-1 text-sm"
						disabled={mutationsDisabled || saving}
					>
						{#if saving}
							<LoaderCircle class="size-4 animate-spin" />
						{/if}
						{currentSecret ? m.core_save() : m.platform_create()}
					</button>
				{/if}
			</div>
		{/if}
	</form>

	{#if form.type === 'ecr'}
		<div class="divider my-0"></div>
		{#if issuerDiscoveryReason}
			<div class="notification-info mt-5 flex items-center gap-3 text-sm">
				<Info class="size-5" />
				<div>
					<p class="font-semibold">{m.platform_settings_image_pull_secrets_issuer_required()}</p>
					<p>{issuerDiscoveryReason}</p>
				</div>
			</div>
		{/if}
		<ECRSetupGuide
			{effectiveIssuerURL}
			{effectiveAudience}
			trustPolicyJSON={previewTrustPolicyJSON}
			ecrPolicyJSON={previewECRPolicyJSON}
		/>
	{/if}
{/if}

{#snippet enabledToggle()}
	<div class="border-base-300 dark:border-base-400 flex items-center gap-1 border-t pt-4 text-sm">
		<Toggle
			label={m.core_status_enabled()}
			labelInline
			checked={form.enabled}
			disabled={mutationsDisabled}
			onChange={(checked) => {
				form.enabled = checked;
			}}
		/>
		<InfoTooltip
			text={m.platform_settings_image_pull_secrets_enabled_help()}
			placement="right"
			class="ml-0.5 size-3.5"
			classes={{ icon: 'size-3.5' }}
		/>
	</div>
{/snippet}

{#snippet basicFields()}
	<div class="flex flex-col gap-4">
		<label class="flex flex-col gap-1">
			<FieldLabel
				label={m.platform_settings_image_pull_secrets_registry_server()}
				help={m.platform_settings_image_pull_secrets_registry_server_help()}
			/>
			<input
				class={inputClass('server')}
				bind:value={form.server}
				disabled={mutationsDisabled}
				placeholder="registry.example.com"
				required
			/>
			{#if requiredErrors.server}
				<span class="text-sm font-medium text-red-500">{requiredErrors.server}</span>
			{/if}
		</label>
		<label class="flex flex-col gap-1">
			<FieldLabel
				label={m.platform_settings_image_pull_secrets_username()}
				help={m.platform_settings_image_pull_secrets_username_help()}
			/>
			<input
				class={inputClass('username')}
				bind:value={form.username}
				disabled={mutationsDisabled}
				placeholder="robot-account"
				required
			/>
			{#if requiredErrors.username}
				<span class="text-sm font-medium text-red-500">{requiredErrors.username}</span>
			{/if}
		</label>
		<label class="flex flex-col gap-1">
			<FieldLabel
				label={m.common_password()}
				help={currentSecret?.status?.passwordConfigured
					? m.platform_settings_image_pull_secrets_password_keep_help()
					: m.platform_settings_image_pull_secrets_password_help()}
			/>
			<input
				class={inputClass('password')}
				type="password"
				bind:value={form.password}
				disabled={mutationsDisabled}
				required={!currentSecret?.status?.passwordConfigured}
				placeholder={currentSecret?.status?.passwordConfigured
					? m.platform_settings_image_pull_secrets_password_keep_placeholder()
					: m.platform_settings_image_pull_secrets_password_placeholder()}
			/>
			{#if requiredErrors.password}
				<span class="text-sm font-medium text-red-500">{requiredErrors.password}</span>
			{/if}
			{#if currentSecret?.status?.passwordConfigured}
				<span class="input-description"
					>{m.platform_settings_image_pull_secrets_password_configured()}</span
				>
			{/if}
		</label>
	</div>
{/snippet}

{#snippet ecrFields()}
	<div class="flex flex-col gap-4">
		<label class="flex flex-col gap-1">
			<FieldLabel
				label={m.platform_settings_image_pull_secrets_role_arn()}
				help={m.platform_settings_image_pull_secrets_role_arn_help()}
			/>
			<input
				class={inputClass('roleARN')}
				bind:value={form.roleARN}
				disabled={mutationsDisabled}
				placeholder="arn:aws:iam::123456789012:role/obot-ecr-pull"
				required
			/>
			{#if requiredErrors.roleARN}
				<span class="text-sm font-medium text-red-500">{requiredErrors.roleARN}</span>
			{/if}
		</label>
		<label class="flex flex-col gap-1">
			<FieldLabel
				label={m.platform_settings_image_pull_secrets_col_region()}
				help={m.platform_settings_image_pull_secrets_region_help()}
			/>
			<input
				class={inputClass('region')}
				bind:value={form.region}
				disabled={mutationsDisabled}
				placeholder="us-east-1"
				required
			/>
			{#if requiredErrors.region}
				<span class="text-sm font-medium text-red-500">{requiredErrors.region}</span>
			{/if}
		</label>

		<div class="flex flex-col gap-4">
			<button
				type="button"
				class="text-muted-content hover:text-base-content flex w-fit items-center gap-1 text-sm font-medium"
				aria-expanded={showECRAdvanced}
				onclick={() => {
					showECRAdvanced = !showECRAdvanced;
				}}
			>
				<ChevronDown
					class={twMerge('size-4 transition-transform', !showECRAdvanced && '-rotate-90')}
				/>
				{m.platform_advanced()}
			</button>

			{#if showECRAdvanced}
				{@render ecrAdvancedFields()}
			{/if}
		</div>
	</div>
{/snippet}

{#snippet ecrAdvancedFields()}
	<div class="flex flex-col gap-4">
		<label class="flex flex-col gap-1">
			<FieldLabel
				label={m.platform_settings_image_pull_secrets_refresh_schedule()}
				help={m.platform_settings_image_pull_secrets_refresh_schedule_help()}
			/>
			<input
				class="input-text-filled"
				bind:value={form.refreshSchedule}
				disabled={mutationsDisabled}
				placeholder="0 */6 * * *"
			/>
		</label>
		<label class="flex flex-col gap-1">
			<FieldLabel
				label={m.platform_settings_image_pull_secrets_issuer_override()}
				help={capability.issuerURL
					? m.platform_settings_image_pull_secrets_issuer_override_optional_help()
					: m.platform_settings_image_pull_secrets_issuer_override_help()}
			/>
			<input
				class="input-text-filled"
				bind:value={form.issuerURL}
				disabled={mutationsDisabled}
				placeholder="https://obot.example.com"
			/>
		</label>
		<label class="flex flex-col gap-1">
			<FieldLabel
				label={m.platform_settings_image_pull_secrets_audience()}
				help={m.platform_settings_image_pull_secrets_audience_help()}
			/>
			<input
				class="input-text-filled"
				bind:value={form.audience}
				disabled={mutationsDisabled}
				placeholder={defaultECRAudience}
			/>
		</label>
	</div>
{/snippet}
