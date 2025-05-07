<script lang="ts">
	import { closeSidebarConfig, getLayout } from '$lib/context/layout.svelte';
	import type { ProjectMCP } from '$lib/services';
	import { isValidMcpConfig, type MCPServerInfo } from '$lib/services/chat/mcp';
	import { ChevronsRight, Plus, Server, Trash2, X } from 'lucide-svelte';
	import { twMerge } from 'tailwind-merge';
	import HostedMcpForm from './HostedMcpForm.svelte';

	interface Props {
		projectMcp?: ProjectMCP;
		onCreate?: (customMcpServerInfo: MCPServerInfo) => void;
		onUpdate?: (customMcpServerInfo: MCPServerInfo) => void;
	}
	let { projectMcp, onCreate, onUpdate }: Props = $props();

	function isObotHosted(projectMcp: ProjectMCP) {
		return (
			(projectMcp.env?.length ?? 0) > 0 ||
			(projectMcp.args?.length ?? 0) > 0 ||
			!!projectMcp.command
		);
	}

	const initConfig: MCPServerInfo = projectMcp
		? {
				...projectMcp,
				env:
					projectMcp.env?.map((env) => ({
						...env,
						value: '' // TODO: update once Donnie gets the /reveal endpoint in
					})) ?? [],
				headers:
					projectMcp.headers?.map((header) => ({
						...header,
						value: '' // TODO: update once Donnie gets the /reveal endpoint in
					})) ?? []
			}
		: {
				description: '',
				icon: '',
				name: '',
				env: [],
				args: [],
				command: '',
				url: '',
				headers: []
			};

	let config = $state<MCPServerInfo>({ ...initConfig });
	let showObotHosted = $state(projectMcp ? isObotHosted(projectMcp) : true);
	let showSubmitError = $state(false);
	const layout = getLayout();

	function init(isRemote?: boolean) {
		config = {
			...initConfig
		};
		showObotHosted = !isRemote;
	}

	async function handleSubmit() {
		if (!isValidMcpConfig(config)) {
			showSubmitError = true;
			return;
		}
		if (projectMcp) {
			onUpdate?.(config);
		} else {
			onCreate?.(config);
		}
	}

	function focusOnAdd(node: HTMLInputElement, shouldFocus: boolean) {
		if (shouldFocus) {
			node.focus();
		}
	}
</script>

<div class="flex h-full w-full flex-col">
	<div class="flex min-h-0 grow flex-col">
		<div class="mt-8 flex w-full flex-col gap-2 px-4 md:px-8">
			<div class="flex w-full flex-col gap-2 self-center md:max-w-[900px]">
				<div class="flex items-center justify-between gap-8">
					{#if projectMcp}
						<div class="flex flex-col gap-4 p-4 md:p-6">
							<div class="flex max-w-sm items-center gap-2">
								<div
									class="h-fit flex-shrink-0 self-start rounded-md bg-gray-50 p-1 dark:bg-gray-600"
								>
									{#if projectMcp.icon}
										<img src={projectMcp.icon} alt={projectMcp.name} class="size-6" />
									{:else}
										<Server class="size-6" />
									{/if}
								</div>
								<div class="flex flex-col gap-1">
									<h3 class="text-lg leading-4.5 font-semibold">
										{projectMcp.name || 'My Custom Server'}
									</h3>
								</div>
							</div>
							<p class="text-sm font-light text-gray-500">
								{projectMcp.description}
							</p>
						</div>
					{:else}
						<h3 class="text-xl font-semibold">Create MCP Config</h3>
					{/if}
					<div class="flex gap-2">
						<button class="icon-button" onclick={() => closeSidebarConfig(layout)}>
							<X class="size-6" />
						</button>
					</div>
				</div>
			</div>
		</div>

		{#if !projectMcp?.catalogID}
			<div
				class="dark:bg-gray-980 mt-4 flex w-full flex-col gap-2 bg-gray-50 px-4 pt-4 pb-2 shadow-inner md:px-8"
			>
				<div class="flex w-full self-center md:max-w-[900px]">
					<div class="flex w-full gap-1">
						<button
							class={twMerge(
								'dark:bg-gray-980 flex-1 bg-gray-50 py-3',
								showObotHosted &&
									'dark:bg-surface2 dark:border-surface3 rounded-md bg-white shadow-sm dark:border'
							)}
							onclick={() => init()}
						>
							Obot Hosted
						</button>
						<button
							class={twMerge(
								'dark:bg-gray-980 flex-1 bg-gray-50 py-3',
								!showObotHosted &&
									'dark:bg-surface2 dark:border-surface3 rounded-md bg-white shadow-sm dark:border'
							)}
							onclick={() => init(true)}
						>
							Remote
						</button>
					</div>
				</div>
			</div>
		{/if}

		<div
			class="default-scrollbar-thin dark:bg-gray-980 relative flex grow flex-col gap-8 overflow-y-auto bg-gray-50 px-4 pb-4 md:px-8"
			class:pt-4={projectMcp?.catalogID}
		>
			<div
				class="dark:bg-surface2 dark:border-surface3 flex w-full grow flex-col gap-4 self-center rounded-lg bg-white px-4 pt-12 pb-8 shadow-sm md:max-w-[900px] md:px-8 dark:border"
			>
				{#if showObotHosted}
					<HostedMcpForm bind:config {showSubmitError} />
				{:else}
					{@render remoteHostedConfig()}
				{/if}
			</div>
		</div>

		<div class="flex w-full flex-col gap-8 px-4 md:px-8">
			<div
				class="flex w-full flex-col justify-end gap-4 self-center rounded-sm py-4 md:max-w-[900px]"
			>
				<button class="button-primary flex items-center gap-1 self-end" onclick={handleSubmit}>
					{projectMcp ? 'Update' : 'Configure'} server <ChevronsRight class="size-4" />
				</button>
			</div>
		</div>
	</div>
</div>

{#snippet remoteHostedConfig()}
	<div class="flex items-center gap-4">
		<h4 class="w-24 text-base font-semibold">URL</h4>
		<input class="text-input-filled flex grow" bind:value={config.command} />
	</div>
	<div class="flex flex-col gap-2">
		<h4 class="text-base font-semibold">Headers</h4>
		{#if config.headers && config.headers.length > 0}
			{#each config.headers as header, i}
				<div class="flex w-full items-center gap-2">
					<div class="flex grow flex-col gap-1">
						<input
							class="ghost-input w-full py-0 pl-1"
							bind:value={header.key}
							placeholder="Key (ex. API_KEY)"
							use:focusOnAdd={i === config.headers.length - 1}
						/>
						<input
							data-1p-ignore
							id={header.name}
							name={header.name}
							class="text-input-filled w-full"
							bind:value={header.value}
							type={header.sensitive ? 'password' : 'text'}
						/>
					</div>
					<button class="icon-button" onclick={() => config.headers?.splice(i, 1)}>
						<Trash2 class="size-4" />
					</button>
				</div>
			{/each}
		{/if}
		<div class="flex justify-end pt-2">
			<button
				class="button flex items-center gap-1 text-xs"
				onclick={() =>
					config.headers?.push({
						name: '',
						key: '',
						description: '',
						sensitive: false,
						required: false,
						file: false,
						value: ''
					})}
			>
				<Plus class="size-4" /> Header
			</button>
		</div>
	</div>
{/snippet}
