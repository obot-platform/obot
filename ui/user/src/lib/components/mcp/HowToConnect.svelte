<script lang="ts">
	import { m } from '$lib/i18n';
	import {
		AiClient,
		COMMAND_SUPPORTED_AI_CLIENTS,
		COMMON_AI_CLIENTS,
		MAGIC_LINK_SUPPORTED_AI_CLIENTS
	} from '$lib/services/user/constants';
	import { getAiClientCommand, getAiClientMagicLink } from '$lib/services/user/mcp';
	import { userDeviceSettings } from '$lib/stores';
	import CopyField from '../CopyField.svelte';
	import { CircleCheckBig } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		id: string;
		displayName: string;
		url: string;
		onLaunch?: () => void;
		onEdit?: () => void;
		onReauthenticate?: () => void;
	}

	let { id, displayName, url, onLaunch, onEdit, onReauthenticate }: Props = $props();

	let aiClientsMap = $derived(new Map(COMMON_AI_CLIENTS.map((client) => [client.id, client])));
	let magicLinks = $derived(generateMcpLinks(displayName, url));
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

	function generateMcpLinks(displayName: string, connectUrl: string) {
		const prefs = userDeviceSettings.aiClientPreference ?? [];
		const preferred = prefs.length ? new Set(prefs) : new Set(MAGIC_LINK_SUPPORTED_AI_CLIENTS);
		return MAGIC_LINK_SUPPORTED_AI_CLIENTS.filter((client) => preferred.has(client)).map(
			(client) => ({
				client,
				link: getAiClientMagicLink(client, displayName, connectUrl)
			})
		);
	}

	function generateAiClientCommands(id: string, url: string) {
		const prefs = userDeviceSettings.aiClientPreference ?? [];
		const preferred = prefs.length ? new Set(prefs) : new Set(COMMAND_SUPPORTED_AI_CLIENTS);
		return COMMAND_SUPPORTED_AI_CLIENTS.filter((client) => preferred.has(client)).map((client) => ({
			client,
			command: getAiClientCommand(client, id, url)
		}));
	}

	export function resetCopied() {
		copyFields.forEach((copyField) => {
			copyField?.clear();
		});
	}
</script>

<div class="w-full @container md:px-0 px-4">
	{#if magicLinks.length > 0}
		<div class="divider">{m.mcps_connect_quick_install()}</div>
		<div
			id="magic-links-container"
			class={twMerge('flex gap-2 flex-col', commands.length > 0 ? 'mb-8' : '')}
		>
			{#each magicLinks as magicLink (magicLink.client)}
				{@const client = aiClientsMap.get(magicLink.client as AiClient)}
				{#if client && magicLink.link}
					<div
						id={`magic-link-${magicLink.client.toLowerCase()}-container`}
						class="rounded-field bg-base-200 shadow-inner border-none input gap-0 w-full px-0 overflow-y-hidden"
					>
						<div
							class="rounded-l-field label w-43 px-2.5 flex items-center gap-2 text-xs text-base-content/75 shrink-0 ml-1 mr-0 bg-base-100 dark:bg-base-300"
						>
							<img
								src={client?.iconDark ?? client?.icon}
								alt={m.mcps_connect_branding_icon_alt({ name: client?.alt ?? '' })}
								class="size-4 dark:block hidden"
							/>
							<img
								src={client?.icon}
								alt={m.mcps_connect_branding_icon_alt({ name: client?.alt ?? '' })}
								class="size-4 block dark:hidden"
							/>
							{client?.alt}
						</div>
						<div class="grow flex mr-1 relative">
							<a
								id={`magic-link-${magicLink.client.toLowerCase()}`}
								href={magicLink.link}
								rel="noopener noreferrer external"
								class="h-8 flex gap-2 justify-center font-mono uppercase items-center text-xs btn btn-secondary hover:bg-primary hover:text-primary-content mx-2 grow"
							>
								<img
									src={client?.iconDark ?? client?.icon}
									alt={m.mcps_connect_branding_icon_alt({ name: client?.alt ?? '' })}
									class="size-4 dark:block hidden"
								/>
								<img
									src={client?.icon}
									alt={m.mcps_connect_branding_icon_alt({ name: client?.alt ?? '' })}
									class="size-4 block dark:hidden"
								/>
								{m.mcps_connect_add_to({ name: client?.alt ?? '' })}
							</a>
						</div>
					</div>
				{/if}
			{/each}
		</div>
	{/if}

	{#if commands.length > 0}
		<div class="divider">{m.mcps_connect_install_via_cli()}</div>
		<div id="cli-commands-container" class="flex gap-2 flex-col">
			{#each commands as aiClientCommand, index (aiClientCommand.client)}
				{@const client = aiClientsMap.get(aiClientCommand.client as AiClient)}
				{#if client && aiClientCommand.command}
					<div id={`command-${aiClientCommand.client}-container`}>
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
										alt={m.mcps_connect_branding_icon_alt({ name: client?.alt ?? '' })}
										class="size-4 dark:block hidden"
									/>
									<img
										src={client?.icon}
										alt={m.mcps_connect_branding_icon_alt({ name: client?.alt ?? '' })}
										class="size-4 block dark:hidden"
									/>
									{client?.alt}
								</span>
							{/snippet}
						</CopyField>
					</div>
				{/if}
			{/each}
		</div>
	{/if}

	{#if onLaunch || onEdit || onReauthenticate}
		{#if onLaunch}
			<div class={twMerge('divider', commands.length > 0 ? 'mt-8' : '')}>
				{m.mcps_connect_preconfigure()}
			</div>
			<p class="text-xs text-center">
				{m.mcps_connect_preconfigure_prefix()}<button
					class="text-blue-500 underline hover:text-blue-400"
					aria-label={m.mcps_connect_preconfigure_server()}
					onclick={onLaunch}>{m.mcps_connect_click_here()}</button
				>{m.mcps_connect_preconfigure_suffix()}
			</p>
		{:else if onEdit || onReauthenticate}
			<div class={twMerge('divider', commands.length > 0 ? 'mt-8' : '')}>
				<span>
					{m.mcps_connect_preconfigure()}
					<CircleCheckBig class="size-4 text-primary shrink-0 inline-block" />
				</span>
			</div>
			<div role="status" class="notification-info text-xs text-center">
				{m.mcps_connect_already_configured()}
				{#if onEdit}
					{m.mcps_connect_update_prefix()}<button
						class="text-blue-500 underline hover:text-blue-400"
						aria-label={m.mcps_connect_edit_configuration()}
						onclick={onEdit}>{m.mcps_connect_click_here()}</button
					>{m.mcps_connect_update_suffix()}
				{:else if onReauthenticate}
					{m.mcps_connect_reauth_prefix()}<button
						class="text-blue-500 underline hover:text-blue-400"
						aria-label={m.mcps_connect_reauthenticate()}
						onclick={onReauthenticate}>{m.mcps_connect_click_here()}</button
					>{m.mcps_connect_reauth_suffix()}
				{/if}
			</div>
		{/if}
	{/if}
</div>
<div class="divider mb-2"></div>
<div class="w-full px-4 md:px-0">
	<div class="flex flex-col md:flex-row w-full gap-2 md:justify-end justify-center items-center">
		<p class="text-xs font-light text-muted-content">
			{m.mcps_connect_more_docs()}
		</p>
		<div class="flex gap-2 items-center justify-end">
			{#each options as option (option.id)}
				<a
					href={option.url}
					target="_blank"
					rel="noopener noreferrer external"
					class="tooltip tooltip-left shrink-0"
					data-tip={option.label}
					aria-label={m.mcps_connect_open_docs({ name: option.label })}
				>
					<img
						src={option.icon}
						alt={m.mcps_connect_branding_icon_alt({ name: option.label })}
						class="size-4"
					/>
				</a>
			{/each}
		</div>
	</div>
</div>
