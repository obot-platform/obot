<script lang="ts">
	import {
		type AppNotification,
		type GitCredential,
		type ImagePullSecret,
		type ImagePullSecretCapability,
		type ModelProxySettings,
		type ModelProxyUsage,
		type ProductTelemetryConsent
	} from '$lib/services';
	import { profile } from '$lib/stores';
	import GitCredentialsView from './GitCredentialsView.svelte';
	import ModelProxyView from './ModelProxyView.svelte';
	import NotificationsView from './NotificationsView.svelte';
	import ProductAnalyticsView from './ProductAnalyticsView.svelte';
	import RegistryConnectionsView from './RegistryConnectionsView.svelte';
	import { Plus } from '@lucide/svelte';
	import type { Snippet } from 'svelte';

	interface Props {
		appNotification: AppNotification;
		capability: ImagePullSecretCapability;
		gitCredentials: GitCredential[];
		imagePullSecrets: ImagePullSecret[];
		modelProxySettings?: ModelProxySettings;
		modelProxyUsage?: ModelProxyUsage;
		productTelemetryConsent?: ProductTelemetryConsent;
		showProductAnalytics?: boolean;
		showRegistryConnections?: boolean;
	}

	let {
		appNotification,
		capability = $bindable(),
		gitCredentials = $bindable(),
		imagePullSecrets = $bindable(),
		modelProxySettings,
		modelProxyUsage,
		productTelemetryConsent,
		showProductAnalytics = false,
		showRegistryConnections = false
	}: Props = $props();

	let notificationsView = $state<ReturnType<typeof NotificationsView>>();
	let productAnalyticsView = $state<ReturnType<typeof ProductAnalyticsView>>();
	let modelProxyView = $state<ReturnType<typeof ModelProxyView>>();
	let registryView = $state<ReturnType<typeof RegistryConnectionsView>>();
	let gitCredentialsView = $state<ReturnType<typeof GitCredentialsView>>();

	let notificationsDirty = $state(false);
	let productAnalyticsDirty = $state(false);
	let modelProxyDirty = $state(false);
	let registryDirty = $state(false);
	let gitCredentialsDirty = $state(false);
	let saving = $state(false);

	let isAdminReadonly = $derived(profile.current.isAdminReadonly?.());
	let isDirty = $derived(
		notificationsDirty ||
			productAnalyticsDirty ||
			modelProxyDirty ||
			registryDirty ||
			gitCredentialsDirty
	);
	let canCreateGitCredential = $derived(!isAdminReadonly);

	const descriptions: Record<string, string | Snippet> = {
		notifications:
			'Set up a notification banner to display at the top of the application across all pages.',
		productAnalytics: productAnalyticsSnippet,
		modelProxy:
			'Configure whether or not the provided model proxy should be enabled to use for the vMCP Inspector.',
		registryConnections:
			'Create a managed image pull secret to let Obot pull private MCP server images.',
		gitCredentials:
			'Create a host-bound credential to use a personal access token across Git repositories.'
	};

	async function handleSave(event: SubmitEvent) {
		event.preventDefault();
		if (
			saving ||
			!isDirty ||
			isAdminReadonly ||
			(event.submitter instanceof HTMLElement && event.submitter.dataset.settingsSave !== 'true')
		) {
			return;
		}
		if (notificationsDirty && notificationsView && !notificationsView.validate()) return;
		if (registryDirty && registryView && !registryView.validate()) return;

		saving = true;
		try {
			await Promise.all([
				notificationsDirty ? notificationsView?.save() : undefined,
				productAnalyticsDirty ? productAnalyticsView?.save() : undefined,
				registryDirty ? registryView?.save() : undefined,
				gitCredentialsDirty ? gitCredentialsView?.save() : undefined
			]);
			if (modelProxyDirty) {
				await modelProxyView?.save();
			}
		} finally {
			saving = false;
		}
	}

	function handleCancel() {
		notificationsView?.reset();
		productAnalyticsView?.reset();
		modelProxyView?.reset();
		registryView?.reset();
		gitCredentialsView?.reset();
	}
</script>

<div class="flex w-full flex-col gap-4">
	<form id="platform-settings" class="flex flex-col gap-4" novalidate onsubmit={handleSave}>
		{@render section('Notifications', 'notifications', descriptions.notifications, notifications)}
		{#if showProductAnalytics}
			{@render section(
				'Product Analytics',
				'product-analytics',
				descriptions.productAnalytics,
				productAnalytics
			)}
		{/if}
		{#if modelProxySettings}
			{@render section('Model Proxy', 'model-proxy', descriptions.modelProxy, modelProxy)}
		{/if}
	</form>

	{#if showRegistryConnections}
		{@render section(
			'Registry Connections',
			'registry-connections',
			descriptions.registryConnections,
			registryConnections
		)}
	{/if}

	{@render section(
		'Git Credentials',
		'git-credentials',
		descriptions.gitCredentials,
		gitCredentialsSection,
		gitCredentialAction
	)}

	{#if !isAdminReadonly}
		<div
			class="bg-base-200 dark:bg-base-100 sticky bottom-0 left-0 flex w-[calc(100%+2em)] -translate-x-4 items-center justify-end gap-4 p-4 md:w-[calc(100%+4em)] md:-translate-x-8 md:px-8 z-40"
		>
			<button
				type="button"
				class="btn btn-secondary text-sm"
				onclick={handleCancel}
				disabled={saving || !isDirty}
			>
				Cancel
			</button>
			<button
				type="submit"
				form="platform-settings"
				class="btn btn-primary text-sm"
				data-settings-save="true"
				disabled={saving || !isDirty}
			>
				Save
			</button>
		</div>
	{/if}
</div>

{#snippet section(
	title: string,
	id: string,
	description: string | Snippet,
	content: Snippet,
	action?: Snippet,
	skipDivider?: boolean
)}
	<section {id} class="flex scroll-mt-24 flex-col">
		<div class="flex flex-wrap items-center justify-between gap-3 mb-4">
			<div>
				<h2 class="text-lg font-semibold">{title}</h2>
				{#if typeof description === 'string'}
					<p class="text-muted-content text-sm font-light">{description}</p>
				{:else}
					{@render description()}
				{/if}
			</div>
			{#if action}
				{@render action()}
			{/if}
		</div>
		{@render content()}
		{#if !skipDivider}
			<div class="divider mb-0 mt-6"></div>
		{/if}
	</section>
{/snippet}

{#snippet notifications()}
	<NotificationsView
		bind:this={notificationsView}
		bind:dirty={notificationsDirty}
		{appNotification}
		{saving}
	/>
{/snippet}

{#snippet productAnalytics()}
	<ProductAnalyticsView
		bind:this={productAnalyticsView}
		bind:dirty={productAnalyticsDirty}
		consent={productTelemetryConsent ?? {}}
		{saving}
	/>
{/snippet}

{#snippet modelProxy()}
	{#if modelProxySettings}
		<ModelProxyView
			bind:this={modelProxyView}
			bind:dirty={modelProxyDirty}
			settings={modelProxySettings}
			usage={modelProxyUsage}
			{saving}
		/>
	{/if}
{/snippet}

{#snippet productAnalyticsSnippet()}
	<p class="text-muted-content text-sm font-light">
		Share product usage data to help improve Obot.
		<a
			class="text-link"
			href="https://docs.obot.ai/configuration/product-analytics"
			target="_blank"
			rel="external noopener noreferrer">Learn more</a
		>
	</p>
{/snippet}

{#snippet registryConnections()}
	<RegistryConnectionsView
		bind:this={registryView}
		bind:dirty={registryDirty}
		bind:capability
		bind:imagePullSecrets
		{saving}
	/>
{/snippet}

{#snippet gitCredentialAction()}
	{#if canCreateGitCredential}
		<button
			type="button"
			class="btn btn-primary flex items-center gap-2 text-sm"
			onclick={() => gitCredentialsView?.openCreate()}
		>
			<Plus class="size-4" />
			Add Git Credential
		</button>
	{/if}
{/snippet}

{#snippet gitCredentialsSection()}
	<GitCredentialsView
		bind:this={gitCredentialsView}
		bind:dirty={gitCredentialsDirty}
		bind:gitCredentials
		{saving}
	/>
{/snippet}
