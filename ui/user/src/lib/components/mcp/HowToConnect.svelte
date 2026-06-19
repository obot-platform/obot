<script lang="ts">
	import {
		AiClient,
		COMMAND_SUPPORTED_AI_CLIENTS,
		COMMON_AI_CLIENTS,
		MAGIC_LINK_SUPPORTED_AI_CLIENTS
	} from '$lib/services/user/constants';
	import { getAiClientCommand, getAiClientMagicLink } from '$lib/services/user/mcp';
	import { userDeviceSettings } from '$lib/stores';
	import CopyField from '../CopyField.svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		id: string;
		url: string;
	}

	let { url, id }: Props = $props();

	let aiClientsMap = $derived(new Map(COMMON_AI_CLIENTS.map((client) => [client.id, client])));
	let magicLinks = $derived(generateMcpLinks(id, url));
	let commands = $derived(generateAiClientCommands(id, url));

	let copyFields = $state<ReturnType<typeof CopyField>[]>([]);

	const options = [
		{
			id: 'cursor',
			label: 'Cursor',
			icon: '/user/images/assistant/cursor-mark.svg',
			url: 'https://cursor.com/docs/mcp'
		},
		{
			id: 'claudeDesktop',
			label: 'Claude Desktop',
			icon: '/user/images/assistant/claude-mark.svg',
			url: 'https://support.claude.com/en/articles/11175166-get-started-with-custom-connectors-using-remote-mcp'
		},
		{
			id: 'claudeCode',
			label: 'Claude Code',
			icon: '/user/images/assistant/claude-mark.svg',
			url: 'https://code.claude.com/docs/en/mcp'
		},
		{
			id: 'vscode',
			label: 'VSCode',
			icon: '/user/images/assistant/vscode-mark.svg',
			url: 'https://code.visualstudio.com/docs/copilot/customization/mcp-servers'
		}
	];

	function generateMcpLinks(id: string, connectUrl: string) {
		const userSetPreferences = new Set(userDeviceSettings.aiClientPreference ?? []);
		return MAGIC_LINK_SUPPORTED_AI_CLIENTS.filter((client) => userSetPreferences.has(client)).map(
			(client) => ({
				client,
				link: getAiClientMagicLink(client, id, connectUrl)
			})
		);
	}

	function generateAiClientCommands(id: string, url: string) {
		const userSetPreferences = new Set(userDeviceSettings.aiClientPreference ?? []);
		return COMMAND_SUPPORTED_AI_CLIENTS.filter((client) => userSetPreferences.has(client)).map(
			(client) => ({
				client,
				command: getAiClientCommand(client, id, url)
			})
		);
	}

	export function resetCopied() {
		copyFields.forEach((copyField) => {
			copyField.clear();
		});
	}
</script>

<div class="w-full @container md:px-0 px-4">
	{#if magicLinks.length > 0}
		<div class="divider">Quick Install</div>
		<div class={twMerge('flex gap-2 flex-col', commands.length > 0 ? 'mb-8' : '')}>
			{#each magicLinks as magicLink (magicLink.client)}
				{@const client = aiClientsMap.get(magicLink.client as AiClient)}
				{#if client && magicLink.link}
					<div
						class="rounded-field bg-base-200 shadow-inner border-none input gap-0 w-full px-0 overflow-y-hidden"
					>
						<div
							class="rounded-l-field label w-43 px-2.5 flex items-center gap-2 text-xs text-base-content/75 shrink-0 ml-1 mr-0 bg-base-100 dark:bg-base-300"
						>
							<img
								src={client?.iconDark ?? client?.icon}
								alt={`${client?.alt} branding icon`}
								class="size-4 dark:block hidden"
							/>
							<img
								src={client?.icon}
								alt={`${client?.alt} branding icon`}
								class="size-4 block dark:hidden"
							/>
							{client?.alt}
						</div>
						<div class="grow flex mr-1 relative">
							<a
								href={magicLink.link}
								rel="noopener noreferrer external"
								class="h-8 flex gap-2 justify-center font-mono uppercase items-center text-xs btn btn-secondary hover:bg-primary hover:text-primary-content mx-2 grow"
							>
								<img
									src={client?.iconDark ?? client?.icon}
									alt={`${client?.alt} branding icon`}
									class="size-4 dark:block hidden"
								/>
								<img
									src={client?.icon}
									alt={`${client?.alt} branding icon`}
									class="size-4 block dark:hidden"
								/>
								Add to {client?.alt}
							</a>
						</div>
					</div>
				{/if}
			{/each}
		</div>
	{/if}

	{#if commands.length > 0}
		<div class="divider">Install via CLI</div>
		<div class="flex gap-2 flex-col">
			{#each commands as aiClientCommand, index (aiClientCommand.client)}
				{@const client = aiClientsMap.get(aiClientCommand.client as AiClient)}
				{#if client && aiClientCommand.command}
					<CopyField
						value={aiClientCommand.command}
						id={`command-${aiClientCommand.client}`}
						classes={{
							inputLabel: 'bg-base-100 dark:bg-base-300',
							input: 'font-mono'
						}}
						bind:this={copyFields[index]}
					>
						{#snippet preContent()}
							<span class="label shrink-0 w-38 mr-0 text-base-content">
								<img
									src={client?.iconDark ?? client?.icon}
									alt={`${client?.alt} branding icon`}
									class="size-4 dark:block hidden"
								/>
								<img
									src={client?.icon}
									alt={`${client?.alt} branding icon`}
									class="size-4 block dark:hidden"
								/>
								{client?.alt}
							</span>
						{/snippet}
					</CopyField>
				{/if}
			{/each}
		</div>
	{/if}
</div>
<div class="divider mb-2"></div>
<div class="w-full px-4 md:px-0">
	<div class="flex flex-col md:flex-row w-full gap-2 md:justify-end justify-center items-center">
		<p class="text-xs font-light text-muted-content">
			For more documentation on how to set up your MCP server:
		</p>
		<div class="flex gap-2 items-center justify-end">
			{#each options as option (option.id)}
				<a
					href={option.url}
					target="_blank"
					rel="noopener noreferrer external"
					class="tooltip tooltip-left shrink-0"
					data-tip={option.label}
					aria-label={`Open ${option.label} MCP server documentation`}
				>
					<img src={option.icon} alt={`${option.label} branding icon`} class="size-4" />
				</a>
			{/each}
		</div>
	</div>
</div>
