<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import { dialogAnimation } from '$lib/actions/dialogAnimation';
	import { formatNumber } from '$lib/format';
	import {
		ChatService,
		type ProjectCredential,
		type MCPManifest,
		type Project,
		type ProjectMCP,
		EditorService
	} from '$lib/services';
	import { darkMode, responsive } from '$lib/stores';
	import { ChevronRight, ChevronsRight, Info, LoaderCircle, Server, Star, X } from 'lucide-svelte';
	import {
		initConfigFromManifest,
		isValidMcpConfig,
		type MCPServerInfo
	} from '$lib/services/chat/mcp';
	import HostedMcpForm from '$lib/components/mcp/HostedMcpForm.svelte';
	import type { Snippet } from 'svelte';
	import CredentialAuth from '$lib/components/edit/CredentialAuth.svelte';
	import RemoteMcpForm from './RemoteMcpForm.svelte';

	interface Props {
		manifest?: MCPManifest | ProjectMCP;
		prefilledConfig?: MCPServerInfo;
		disableOutsideClick?: boolean;
		hideCloseButton?: boolean;
		onUpdate?: (manifest: MCPServerInfo) => void;
		selected?: boolean;
		submitText?: string;
		leftActionContent?: Snippet;
		children?: Snippet;
		legacyBundleId?: string;
		project?: Project;
		legacyAuthText?: string;
		manifestType?: 'command' | 'url';
	}
	let {
		manifest,
		prefilledConfig,
		disableOutsideClick,
		hideCloseButton,
		onUpdate,
		selected,
		submitText,
		leftActionContent,
		children,
		legacyBundleId,
		legacyAuthText,
		project = $bindable(),
		manifestType
	}: Props = $props();
	let configDialog = $state<HTMLDialogElement>();
	let authDialog = $state<HTMLDialogElement>();

	let config = $state<MCPServerInfo>(prefilledConfig ?? initConfigFromManifest(manifest));
	let showSubmitError = $state(false);
	let loadingCredential = $state<Promise<ProjectCredential | undefined>>();
	export function open() {
		reset();
		configDialog?.showModal();
	}

	function reset() {
		showSubmitError = false;
		config = prefilledConfig ?? initConfigFromManifest(manifest);
	}

	export function close() {
		configDialog?.close();
	}

	function isAuthRequiredBundle(bundleId?: string) {
		if (!bundleId) return false;
		const nonRequiredAuthBundles = [
			'browser-bundle',
			'google-search-bundle',
			'images-bundle',
			'memory',
			'obot-search',
			'time',
			'database',
			'die-roller',
			'proxycurl-bundle' // linkedin search bundle
		];
		return !nonRequiredAuthBundles.includes(bundleId);
	}

	async function getProjectCredential() {
		if (!legacyBundleId) return;

		if (!project) {
			project = await EditorService.createObot();
		}

		const response = (
			await ChatService.listProjectLocalCredentials(project.assistantID, project.id)
		).items;
		const credential = response.find((cred) => cred.toolID === legacyBundleId);

		if (credential?.exists) {
			// delete the credential if it exists
			// user is choosing to re-authenticate
			await ChatService.deleteProjectLocalCredential(
				project.assistantID,
				project.id,
				credential.toolID
			);
		}

		return credential
			? {
					...credential,
					exists: false
				}
			: undefined;
	}

	function handleSubmit() {
		if (!manifest) return;

		if (!legacyBundleId && !isValidMcpConfig(config)) {
			showSubmitError = true;
			return;
		}

		if ('server' in manifest) {
			onUpdate?.(config);
		} else {
			onUpdate?.(config);
		}
		close();
	}
</script>

<dialog
	bind:this={configDialog}
	class="default-dialog w-full sm:max-w-lg"
	class:mobile-screen-dialog={responsive.isMobile}
	use:clickOutside={() => {
		if (disableOutsideClick) return;
		close();
	}}
	use:dialogAnimation={{ type: 'fade' }}
>
	<div class="grid h-fit max-h-[calc(100vh-4rem)] grid-rows-[auto_1fr_auto]">
		{@render basicInfo()}
		<div class="default-scrollbar-thin min-h-0 w-full overflow-y-auto px-4 py-1 md:px-6">
			{#if legacyBundleId}
				{#if isAuthRequiredBundle(legacyBundleId)}
					<div class="notification-info mb-4 p-3 text-sm font-light">
						<div class="flex items-center gap-3">
							<Info class="size-6" />
							<p>
								{legacyAuthText ??
									"This server support OAuth authentication. You'll be prompted to login after launching."}
							</p>
						</div>
					</div>
				{/if}
			{:else}
				<div class="flex w-full flex-col gap-4">
					{#if manifestType === 'url'}
						<RemoteMcpForm bind:config {showSubmitError} />
					{:else}
						<HostedMcpForm bind:config {showSubmitError} />
					{/if}
				</div>
			{/if}
		</div>
		<div class="flex items-center justify-between gap-2 px-4 py-4 md:px-6">
			<div>
				{#if leftActionContent}
					{@render leftActionContent()}
				{/if}
			</div>
			<div class="flex-shrink-0">
				<button
					onclick={() => {
						if (legacyBundleId && isAuthRequiredBundle(legacyBundleId)) {
							loadingCredential = getProjectCredential();
							configDialog?.close();
							authDialog?.showModal();
						} else {
							handleSubmit();
						}
					}}
					class="button-primary flex w-full items-center justify-center gap-1 self-end md:w-fit"
				>
					{selected ? 'Update' : (submitText ?? 'Add to Agent')}
					<ChevronsRight class="size-4" />
				</button>
			</div>
		</div>
	</div>
</dialog>

<dialog
	bind:this={authDialog}
	class="default-dialog w-full sm:max-w-lg"
	use:dialogAnimation={{ type: 'fade' }}
>
	{#await loadingCredential}
		<div class="flex w-full items-center justify-center">
			<LoaderCircle class="size-6 animate-spin" />
		</div>
	{:then credential}
		{#if project && legacyBundleId && credential}
			<CredentialAuth
				inline
				local
				toolID={legacyBundleId}
				{project}
				{credential}
				onClose={() => {
					handleSubmit();
					authDialog?.close();
				}}
			/>
		{/if}
	{/await}
</dialog>

{#snippet basicInfo()}
	{#if !hideCloseButton}
		<button class="icon-button absolute top-4 right-4" onclick={() => close()}>
			{#if responsive.isMobile}
				<ChevronRight class="size-6" />
			{:else}
				<X class="size-6" />
			{/if}
		</button>
	{/if}
	{#if manifest}
		{@const icon = 'server' in manifest ? manifest.server.icon : manifest.icon}
		{@const name =
			('server' in manifest ? manifest.server.name : manifest.name) || 'My Custom Server'}
		<div class="flex flex-col gap-4 p-4 md:p-6">
			<div class="flex max-w-sm items-center gap-2">
				<div class="h-fit flex-shrink-0 self-start rounded-md bg-gray-50 p-1 dark:bg-gray-600">
					{#if icon}
						<img src={icon} alt={name} class="size-6" />
					{:else}
						<Server class="size-6" />
					{/if}
				</div>
				<div class="flex flex-col gap-1">
					<h3 class="text-lg leading-5.5 font-semibold">
						{name}
						{#if manifest.url}
							<a
								href={manifest.url}
								target="_blank"
								rel="noopener noreferrer"
								class="ml-1 inline-block align-middle"
							>
								<img
									src={darkMode.isDark
										? '/user/images/github-mark/github-mark-white.svg'
										: '/user/images/github-mark/github-mark.svg'}
									alt="github logo"
									class="size-4 -translate-y-0.25"
								/>
							</a>
						{/if}
					</h3>

					{#if 'githubStars' in manifest}
						<span class="text-md flex h-fit w-fit items-center gap-1 font-light text-gray-500">
							<Star class="size-4" />
							{formatNumber(manifest.githubStars)}
						</span>
					{/if}
				</div>
			</div>
			<p class="text-sm font-light text-gray-500">
				{'server' in manifest ? manifest.server.description : manifest.description}
			</p>
			{#if children}
				{@render children()}
			{/if}
		</div>
	{/if}
{/snippet}
