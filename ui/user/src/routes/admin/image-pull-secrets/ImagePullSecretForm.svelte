<script lang="ts">
	import InfoTooltip from '$lib/components/InfoTooltip.svelte';
	import Select from '$lib/components/Select.svelte';
	import Toggle from '$lib/components/Toggle.svelte';
	import type {
		ImagePullSecret,
		ImagePullSecretCapability,
		ImagePullSecretType
	} from '$lib/services';
	import CapabilityBanner from './CapabilityBanner.svelte';
	import ECRSetupGuide from './ECRSetupGuide.svelte';
	import FieldLabel from './FieldLabel.svelte';
	import {
		defaultECRAudience,
		ecrPolicyJSON,
		ecrTrustPolicyJSON,
		type ImagePullSecretFormState
	} from './types';
	import { ChevronDown, CircleCheck, Info, LoaderCircle, RefreshCw } from 'lucide-svelte';
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
		onSave: () => void;
		onRefresh: (secret: ImagePullSecret) => void;
	}

	const typeOptions: { id: ImagePullSecretType; label: string }[] = [
		{ id: 'basic', label: 'Basic' },
		{ id: 'ecr', label: 'ECR' }
	];

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

<div class="flex flex-col gap-6">
	{#if !capability.available}
		<CapabilityBanner reason={capability.reason} />
	{/if}

	{#if selectedId && !currentSecret}
		<div class="notification-info flex items-center gap-3">
			<Info class="size-5" />
			<div>Image pull secret not found.</div>
		</div>
	{:else}
		<form
			class="paper"
			novalidate
			onsubmit={(e) => {
				e.preventDefault();
				onSave();
			}}
		>
			<div class="flex flex-col gap-4">
				<div class="flex flex-col gap-1">
					<FieldLabel
						label="Type"
						help="Choose Basic for username/password registry credentials, or ECR for AWS IAM role based ECR access."
					/>
					<Select
						id="image-pull-secret-type"
						class="bg-surface1 dark:bg-background dark:border-surface3 border border-transparent shadow-inner"
						classes={{ root: 'w-full' }}
						options={typeOptions}
						selected={form.type}
						disabled={Boolean(currentSecret) || mutationsDisabled}
						onSelect={(option) => {
							form.type = option.id as ImagePullSecretType;
						}}
					/>
				</div>
				<label class="flex flex-col gap-1">
					<FieldLabel
						label="Display Name"
						help="Friendly name shown in the admin list. If omitted, Obot shows the generated secret ID."
					/>
					<input
						class="input-text-filled"
						bind:value={form.displayName}
						disabled={mutationsDisabled}
						placeholder={form.type === 'ecr' ? 'Production ECR access' : 'Production registry'}
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

			<div class="flex flex-wrap items-center justify-end gap-2">
				{#if currentSecret && form.type === 'ecr'}
					<button
						type="button"
						class="button flex items-center gap-1 text-sm"
						disabled={mutationsDisabled || refreshing}
						onclick={() => onRefresh(currentSecret)}
					>
						<RefreshCw class={twMerge('size-4', refreshing && 'animate-spin')} />
						Refresh Now
					</button>
				{/if}
				<button
					type="submit"
					class="button-primary flex items-center gap-1 text-sm"
					disabled={mutationsDisabled || saving}
				>
					{#if saving}
						<LoaderCircle class="size-4 animate-spin" />
					{/if}
					{currentSecret ? 'Save' : 'Create'}
				</button>
			</div>
		</form>

		{#if form.type === 'ecr'}
			{#if issuerDiscoveryReason}
				<div class="notification-info mt-5 flex items-center gap-3 text-sm">
					<Info class="size-5" />
					<div>
						<p class="font-semibold">Issuer URL is required for ECR setup.</p>
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
</div>

{#snippet enabledToggle()}
	<div class="border-surface2 flex items-center gap-1 border-t pt-4 text-sm">
		<Toggle
			label="Enabled"
			labelInline
			checked={form.enabled}
			disabled={mutationsDisabled}
			onChange={(checked) => {
				form.enabled = checked;
			}}
		/>
		<InfoTooltip
			text="Controls whether Obot maintains and uses this managed image pull secret. Disabled secrets remain configured but are not active."
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
				label="Registry Server"
				help="Registry host for the credentials, without an image path, query string, or user info. A scheme is optional."
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
				label="Username"
				help="Registry username or robot account name used with the password or token."
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
				label="Password"
				help={currentSecret?.status?.passwordConfigured
					? 'Leave blank to keep the current stored registry password or token.'
					: 'Registry password, access token, or robot account token. This is stored as a Kubernetes image pull secret.'}
			/>
			<input
				class={inputClass('password')}
				type="password"
				bind:value={form.password}
				disabled={mutationsDisabled}
				required={!currentSecret?.status?.passwordConfigured}
				placeholder={currentSecret?.status?.passwordConfigured
					? 'Leave blank to keep current password'
					: 'Registry password or token'}
			/>
			{#if requiredErrors.password}
				<span class="text-sm font-medium text-red-500">{requiredErrors.password}</span>
			{/if}
			{#if currentSecret?.status?.passwordConfigured}
				<span class="input-description">Password configured</span>
			{/if}
		</label>
	</div>
{/snippet}

{#snippet ecrFields()}
	<div class="flex flex-col gap-4">
		<label class="flex flex-col gap-1">
			<FieldLabel
				label="Role ARN"
				help="AWS IAM role that Obot should assume to request ECR authorization tokens."
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
			<FieldLabel label="Region" help="AWS region that contains the target ECR registry." />
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

		<div class="border-surface2 flex flex-col gap-4 border-t pt-4">
			<button
				type="button"
				class="text-on-surface1 flex w-fit items-center gap-1 text-sm font-medium"
				aria-expanded={showECRAdvanced}
				onclick={() => {
					showECRAdvanced = !showECRAdvanced;
				}}
			>
				<ChevronDown
					class={twMerge('size-4 transition-transform', !showECRAdvanced && '-rotate-90')}
				/>
				Advanced
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
				label="Refresh Schedule"
				help="Cron expression for refreshing the generated ECR image pull secret. Leave blank to use the default, every 6 hours."
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
				label="Issuer URL Override"
				help={capability.issuerURL
					? 'Optional HTTPS OIDC issuer URL to use in the AWS trust policy. Leave blank to use the Obot issuer.'
					: 'HTTPS OIDC issuer URL to use in the AWS trust policy.'}
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
				label="Audience"
				help="Optional OIDC audience value for AWS STS. Leave blank to use sts.amazonaws.com."
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
