<script lang="ts">
	import { AiClient, COMMON_AI_CLIENTS } from '$lib/constants';
	import { encodeUtf8ToBase64 } from '$lib/format';
	import { userDeviceSettings } from '$lib/stores';
	import CopyField from '../CopyField.svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		name: string;
		url: string;
	}

	let { url, name }: Props = $props();

	let aiClientsMap = $derived(new Map(COMMON_AI_CLIENTS.map((client) => [client.id, client])));
	let magicLinks = $derived(generateMcpLinks(name, url));
	let commands = $derived(generateAiClientCommands(url));

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

	const obotApiKeyInput = {
		type: 'promptString',
		id: 'obot-api-key',
		description: 'Obot API Key',
		password: true
	} as const;

	function toVsCodeMcpServerName(displayName: string, connectUrl: string): string {
		const fromUrl = connectUrl.match(/\/mcp-connect\/([^/?#]+)/)?.[1];
		if (fromUrl) {
			return fromUrl;
		}

		const sanitized = displayName
			.trim()
			.replace(/\s+(.)/g, (_, char: string) => char.toUpperCase())
			.replace(/[^a-zA-Z0-9]/g, '');

		return sanitized || 'obotMcpServer';
	}

	function generateMcpLinks(displayName: string, connectUrl: string) {
		const cursorConfig = {
			type: 'http',
			url: connectUrl
		};
		const cursorBase64 = encodeUtf8ToBase64(JSON.stringify(cursorConfig));
		const cursorLink = `cursor://anysphere.cursor-deeplink/mcp/install?name=${encodeURIComponent(displayName)}&config=${cursorBase64}`;

		// VS Code expects the entire query string to be one URL-encoded JSON object.
		const vscodeConfig = {
			name: toVsCodeMcpServerName(displayName, connectUrl),
			inputs: [obotApiKeyInput],
			type: 'http',
			url: connectUrl,
			headers: {
				Authorization: 'Bearer ${input:obot-api-key}'
			}
		};
		const vscodeLink = `vscode:mcp/install?${encodeURIComponent(JSON.stringify(vscodeConfig))}`;

		return {
			...(userDeviceSettings.aiClientPreference?.includes(AiClient.Cursor)
				? { [AiClient.Cursor]: cursorLink }
				: {}),
			...(userDeviceSettings.aiClientPreference?.includes(AiClient.VSCode)
				? { [AiClient.VSCode]: vscodeLink }
				: {})
		};
	}

	function generateAiClientCommands(url: string) {
		return {
			...(userDeviceSettings.aiClientPreference?.includes(AiClient.Claude)
				? { [AiClient.Claude]: `claude mcp add ${url}` }
				: {}),
			...(userDeviceSettings.aiClientPreference?.includes(AiClient.Codex)
				? { [AiClient.Codex]: `codex mcp add ${url}` }
				: {})
		};
	}
</script>

<div class="w-full @container md:px-0 px-4">
	{#if Object.keys(magicLinks).length > 0}
		<div class="divider">Quick Install</div>
		<div class={twMerge('flex gap-2 flex-col', Object.keys(commands).length > 1 ? 'mb-4' : '')}>
			{#each Object.keys(magicLinks) as clientId (clientId)}
				{@const client = aiClientsMap.get(clientId as AiClient)}
				{@const link = magicLinks[clientId as keyof typeof magicLinks]}

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
							href={link}
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
			{/each}
		</div>
	{/if}

	{#if Object.keys(commands).length > 0}
		<div class="divider">Install via CLI</div>
		<div class="flex gap-2 flex-col">
			{#each Object.keys(commands) as clientId (clientId)}
				{@const client = aiClientsMap.get(clientId as AiClient)}
				{@const command = commands[clientId as keyof typeof commands]}

				<CopyField
					value={command}
					id={`command-${clientId}`}
					classes={{
						inputLabel: 'bg-base-100 dark:bg-base-300',
						input: 'font-mono'
					}}
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
