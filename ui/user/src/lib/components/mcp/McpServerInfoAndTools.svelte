<script lang="ts">
	import {
		ChatService,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type Project,
		type ProjectMCP
	} from '$lib/services';
	import { twMerge } from 'tailwind-merge';
	import McpServerInfo from './McpServerInfo.svelte';
	import McpServerTools from './McpServerTools.svelte';
	import McpOauth from './McpOauth.svelte';
	import { AlertTriangle } from 'lucide-svelte';
	import CatalogConfigureForm, {
		type CompositeLaunchFormData,
		type LaunchFormData
	} from './CatalogConfigureForm.svelte';
	import {
		convertCompositeInfoToLaunchFormData,
		convertCompositeLaunchFormDataToPayload,
		convertEnvHeadersToRecord
	} from '$lib/services/chat/mcp';

	interface Props {
		entry?: MCPCatalogEntry | MCPCatalogServer | ProjectMCP;
		parent?: Props['entry'];
		onAuthenticate?: () => void;
		project?: Project;
		view?: 'overview' | 'tools';
		onProjectToolsUpdate?: (selected: string[]) => void;
		onUpdate?: () => void;
		onEditConfiguration?: () => void;
	}

	let {
		entry,
		parent,
		onAuthenticate,
		project,
		view = 'overview',
		onProjectToolsUpdate,
		onUpdate,
		onEditConfiguration
	}: Props = $props();
	let selected = $derived<string>(view);
	const tabs = [
		{ label: 'Overview', view: 'overview' },
		{ label: 'Tools', view: 'tools' }
	];

	let configDialog = $state<ReturnType<typeof CatalogConfigureForm>>();
	let configureForm = $state<LaunchFormData>();
	let error = $state<string>();
	let saving = $state(false);
	let configuringServer = $state<MCPCatalogServer>();

	async function handleInitCompositeForm(server: MCPCatalogServer) {
		configuringServer = server;
		configureForm = await convertCompositeInfoToLaunchFormData(server);
	}

	async function handleInitSingleOrRemoteForm(server: MCPCatalogServer) {
		let values: Record<string, string>;
		try {
			values = await ChatService.revealSingleOrRemoteMcpServer(server.id, {
				dontLogErrors: true
			});
		} catch (error) {
			if (error instanceof Error && !error.message.includes('404')) {
				console.error('Failed to reveal user server values due to unexpected error', error);
			}
			values = {};
		}
		configuringServer = server;
		configureForm = {
			envs: server.manifest.env?.map((env) => ({
				...env,
				value: values[env.key] ?? ''
			})),
			headers: server.manifest.remoteConfig?.headers?.map((header) => ({
				...header,
				value: values[header.key] ?? '',
				isStatic: header.value !== ''
			})),
			url: server.manifest.remoteConfig?.url
		};
	}

	async function handleInitConfigureForm() {
		if (!entry) return;
		if ('mcpID' in entry) {
			const response = await ChatService.getSingleOrRemoteMcpServer(entry.mcpID);
			if (response?.manifest?.runtime === 'composite') {
				await handleInitCompositeForm(response);
			} else {
				await handleInitSingleOrRemoteForm(response);
			}
			configDialog?.open();
		}
	}

	async function handleConfigureFormUpdate() {
		if (!configuringServer || !configureForm) return;
		try {
			if (
				configuringServer.manifest.runtime === 'remote' &&
				configureForm.url &&
				!configuringServer.manifest.remoteConfig?.isTemplate
			) {
				await ChatService.updateRemoteMcpServerUrl(configuringServer.id, configureForm.url.trim());
			}

			if (configuringServer.manifest.runtime === 'composite') {
				const payload = convertCompositeLaunchFormDataToPayload(
					configureForm as CompositeLaunchFormData
				);

				await ChatService.configureCompositeMcpServer(configuringServer.id, payload);
			} else {
				const secretValues = convertEnvHeadersToRecord(configureForm.envs, configureForm.headers);
				await ChatService.configureSingleOrRemoteMcpServer(configuringServer.id, secretValues);
			}
			configDialog?.close();
			onUpdate?.();
		} catch (error) {
			console.error('Error during configuration:', error);
			configDialog?.close();
		}
	}
</script>

<div class="flex h-full w-full flex-col gap-4">
	<div class="flex grow flex-col gap-2">
		<div class="flex w-full items-center gap-2 px-4">
			<div class="flex gap-2 py-1 text-sm font-light">
				{#each tabs as tab (tab.view)}
					<button
						onclick={() => {
							selected = tab.view;
						}}
						class={twMerge(
							'w-48 flex-shrink-0 rounded-md border border-transparent px-4 py-2 text-center transition-colors duration-300',
							selected === tab.view &&
								'dark:bg-surface1 dark:border-surface3 bg-background shadow-sm',
							selected !== tab.view && 'hover:bg-surface3'
						)}
					>
						{tab.label}
					</button>
				{/each}
			</div>
		</div>

		{#if selected === 'overview' && entry}
			<div class="px-4 pb-8">
				<McpServerInfo
					{entry}
					{parent}
					descriptionPlaceholder="Add a description for this MCP server in the Configuration tab"
				>
					{#snippet preContent()}
						{#if 'configured' in entry && typeof entry.configured === 'boolean' && entry.configured === false}
							<div class="notification-alert mb-4 flex gap-2">
								<div class="flex grow flex-col gap-2">
									<div class="flex items-center gap-2">
										<AlertTriangle class="size-6 flex-shrink-0 self-start text-yellow-500" />
										<p class="my-0.5 flex flex-col text-sm font-semibold">Update Required</p>
									</div>
									<span class="text-sm font-light break-all">
										Due to a recent update in the server, an update on this MCP server's
										configuration is required to continue using this server.
									</span>
								</div>
								<div class="flex flex-shrink-0 items-center">
									<button
										class="button-primary text-sm"
										onclick={() => {
											if (onEditConfiguration) {
												onEditConfiguration();
											} else {
												handleInitConfigureForm();
											}
										}}
									>
										Edit Configuration
									</button>
								</div>
							</div>
						{:else if project}
							<div class="mb-4 w-full">
								<McpOauth {entry} {onAuthenticate} {project} />
							</div>
						{/if}
					{/snippet}
				</McpServerInfo>
			</div>
		{:else if selected === 'tools' && entry}
			<McpServerTools
				{entry}
				{onAuthenticate}
				{project}
				{onProjectToolsUpdate}
				classes={{ root: 'px-4' }}
			/>
		{/if}
	</div>
</div>

<CatalogConfigureForm
	bind:this={configDialog}
	bind:form={configureForm}
	{error}
	icon={configuringServer?.manifest.icon}
	name={configuringServer?.alias || configuringServer?.manifest.name}
	onSave={handleConfigureFormUpdate}
	submitText="Update"
	loading={saving}
/>
