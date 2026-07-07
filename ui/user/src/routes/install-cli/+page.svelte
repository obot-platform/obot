<script lang="ts">
	import { page } from '$app/state';
	import CopyButton from '$lib/components/CopyButton.svelte';
	import Layout from '$lib/components/Layout.svelte';
	import Logo from '$lib/components/Logo.svelte';
	import { COMMON_AI_CLIENTS } from '$lib/services/user/constants';
	import { darkMode, profile } from '$lib/stores';
	import {
		AntennaIcon,
		CheckIcon,
		ComputerIcon,
		ExternalLinkIcon,
		LaptopIcon,
		PcCaseIcon,
		ServerIcon
	} from '@lucide/svelte';

	let isAdmin = $derived(profile.current?.hasAdminAccess?.() ?? false);

	const icons = [
		{
			id: 'computer',
			icon: ComputerIcon,
			color: '#4575b4'
		},
		{
			id: 'laptop',
			icon: LaptopIcon,
			color: '#fdae61'
		},
		{
			id: 'pc-case',
			icon: PcCaseIcon,
			color: '#f46d43'
		},
		{
			id: 'server',
			icon: ServerIcon,
			color: '#74add1'
		}
	];

	const marks = COMMON_AI_CLIENTS;
	const installCommand = 'brew install obot-platform/tap/obot';
	const setupCommand = $derived(`obot setup --url ${page.url.origin}`);
</script>

<Layout title="Obot CLI">
	<div class="w-full max-w-7xl mx-auto h-full @container/cli my-6">
		<div class="paper">
			<div class="flex flex-col @2xl/cli:flex-row items-center justify-center gap-8">
				<div class="max-w-md">
					<Logo class="@2xl/cli:size-16 size-12 mb-1 mx-auto @2xl:mx-0" />
					<h2 class="text-2xl font-bold mb-2">One CLI. Your entire AI tooling.</h2>
					{#if isAdmin}
						<p class="font-light">
							Obot is the open-source platform for hosting, governing, and using Model Context
							Protocol servers & skills. Have users run the CLI locally, and bring your team’s AI
							tooling under one roof.
						</p>
					{:else}
						<p class="font-light">
							Obot is the open-source platform for hosting, governing, and using Model Context
							Protocol servers & skills. Run the CLI locally to start connecting your AI clients and
							have access to your team's AI tooling in one place.
						</p>
					{/if}
				</div>
				<div class="flex grow shrink-0 justify-end">
					{@render mockupCode()}
				</div>
			</div>
		</div>

		<div class="flex flex-col @3xl/cli:flex-row my-12 items-center justify-center">
			<div class="grid grid-cols-4 gap-4 @2xl/cli:min-w-80">
				{#each icons as icon (icon.id)}
					<div
						class="col-span-2 flex justify-center items-center @2xl/cli:size-36 size-24 rounded-md"
						style={`${darkMode.isDark ? 'background-color: color-mix(in oklab, ' + icon.color + ' 20%, transparent);' : 'background-color: color-mix(in oklab, ' + icon.color + ' 10%, transparent);'} color: ${icon.color};`}
					>
						<icon.icon class="@2xl/cli:size-24 size-16" />
					</div>
				{/each}
			</div>
			<div class="p-8 @2xl/cli:pb-8 pb-0 flex flex-col gap-1" id="obot-cli-installation">
				<h3 class="text-2xl font-bold">How to Install Obot CLI</h3>

				{@render codesnippet(
					'For MacOS, install through Homebrew:',
					installCommand,
					'obot-cli-homebrew-install'
				)}

				{@render codesnippet(
					'Then run the following command:',
					setupCommand,
					'obot-cli-setup-command'
				)}

				<div class="flex flex-col">
					<p class="text-sm">For more installation options, click below:</p>
					<a
						id="obot-cli-windows-installer"
						href="https://github.com/obot-platform/obot/releases/latest"
						class="btn btn-primary mb-3 my-2 w-fit self-center @lg/cli:self-start"
						target="_blank"
						rel="noopener noreferrer external"
					>
						Get Latest Release <ExternalLinkIcon class="size-4" />
					</a>
					<div class="flex items-center gap-1 badge badge-outline border-base-400 opacity-50">
						<div class="devicon devicon-windows11-original text-[#0078D7]"></div>
						<p class="text-xs font-light">
							<b class="font-medium">Windows Installer</b> Coming Soon
						</p>
					</div>
				</div>
			</div>
		</div>

		<div class="flex items-center justify-center mb-12">
			<div class="max-w-full w-2xl paper h-full gap-2 py-8">
				<div
					class="p-3 rounded-full dark:bg-base-300 bg-base-200 w-fit justify-center items-center flex self-center"
				>
					<AntennaIcon class="@2xl/cli:size-10 size-6 text-primary translate-y-0.5" />
				</div>
				<h4 class="text-xl font-semibold text-center my-2">
					What does <code class="font-mono font-normal mx-2">obot setup</code> do?
				</h4>
				<ul class="list-disc font-light flex flex-col gap-2 px-4">
					<li>Detects Cursor and Claude Code on your machine</li>
					<li>Authenticates and saves your default Obot URL locally</li>
					<li>
						Installs Obot bootstrap skills so AI clients know how to work with your org’s MCP
						catalog
					</li>
				</ul>
			</div>
		</div>

		<div class="divider"></div>

		<div class="mt-12 flex flex-col gap-4" id="obot-cli-commands">
			<h3 class="text-2xl font-bold">Obot CLI Commands</h3>

			<div class="paper" id="obot-cli-command-setup">
				{@render commandPreview('obot setup')}
				<p>
					Use <code class="inline-code">obot setup</code> to authenticate with Obot and install the Obot
					skills into your AI clients.
				</p>
			</div>

			<div class="paper" id="obot-cli-command-skills">
				{@render commandPreview('obot skills')}
				<p>
					Use <code class="inline-code">obot skills</code> to install and manage skills. We support
					Claude, Codex, and all clients that support <code class="inline-code">~/.agents</code>,
					including:
				</p>
				{@render supportedClients()}

				<ul class="list-disc font-light flex flex-col gap-2 @lg/cli:px-8 px-4 text-sm">
					<li>
						<p class="mb-2">
							Directly install skills to your AI clients using the following skills:
						</p>
						<div class="mb-2">
							{@render slashCommandPreview(
								'/obot-search-skills',
								'Search Obot for installable skills. (user)'
							)}
						</div>
						{@render slashCommandPreview(
							'/obot-install-skill',
							'Install a skill from Obot. (user)'
						)}
					</li>
				</ul>
			</div>

			<div class="paper" id="obot-cli-command-scan">
				{@render commandPreview('obot scan')}
				<div>
					<p class="mb-2">
						Use <code class="inline-code">obot scan</code> to inventory your AI clients, skills, MCP servers,
						and more.
					</p>
					<ul class="list-disc font-light flex flex-col gap-2 @lg/cli:px-8 px-4 text-sm">
						<li>Crawls your home directory for MCP servers, skills, and plugins</li>
						<li>Stable device identity for ongoing visibility</li>
						<li>
							<p class="mb-1">Supported AI Clients:</p>
							{@render supportedClients('size-6')}
						</li>
						<li>
							<p class="mb-2">After reviewing your scan results, submit via:</p>
							<div class="command-preview">
								<pre data-prefix="$" class="m-0"><code>obot scan --submit</code></pre>
							</div>
						</li>
						<li>
							<p class="mb-2">
								Kick off a scan directly from your AI clients using the following skill:
							</p>
							{@render slashCommandPreview(
								'/obot-scan',
								'Scan local AI client configuration and submit it to Obot. (user)'
							)}
						</li>
					</ul>
				</div>
			</div>
		</div>
	</div>
</Layout>

{#snippet codesnippet(step: string, command: string, id: string)}
	<p class="text-sm">{step}</p>
	<div class="relative mt-0.5 mb-4">
		<pre class="pl-4 pr-22 py-2 m-0" {id}><code>{command}</code></pre>
		<div class="absolute top-1/2 right-2 -translate-y-1/2">
			<CopyButton
				text={command}
				id={`${id}-copy-button`}
				classes={{ button: 'flex shrink-0 gap-2 text-xs text-white hover:text-primary' }}
				showTextLeft
			/>
		</div>
	</div>
{/snippet}

{#snippet supportedClients(klass: string = 'size-8')}
	<ul class="flex flex-wrap gap-2 items-center">
		{#each marks as mark (mark.id)}
			<li class="tooltip bg-white dark:bg-base-200" data-tip={mark.alt}>
				{#if darkMode.isDark && mark.iconDark}
					<img src={mark.iconDark} alt={mark.alt} class={klass} />
				{:else}
					<img src={mark.icon} alt={mark.alt} class={klass} />
				{/if}
			</li>
		{/each}
	</ul>
{/snippet}

{#snippet commandPreview(command: string)}
	<div class="command-preview">
		<pre data-prefix="$" class="m-0"><code>{command}</code></pre>
	</div>
{/snippet}

{#snippet slashCommandPreview(command: string, description: string)}
	<div class="command-preview">
		<pre class="m-0"><code
				class="command-line-split flex w-full flex-col gap-1 whitespace-normal px-6 @lg/cli:flex-row @lg/cli:items-baseline @lg/cli:justify-between @lg/cli:gap-4"
				><span class="shrink-0">{command}</span><span
					class="w-full opacity-70 @lg/cli:w-auto @lg/cli:text-right">{description}</span
				></code
			></pre>
	</div>
{/snippet}

{#snippet mockupCode()}
	<div class="mockup-code w-full max-w-2xl">
		<pre></pre>
		<pre data-prefix="$"><code>obot setup</code></pre>
		<pre data-prefix=">"><code
				><CheckIcon class="text-success size-4 inline" /> Connected Cursor and Claude Code</code
			></pre>
		<pre></pre>
		<pre data-prefix="$"><code>obot scan</code></pre>
		<pre data-prefix=">"><code>27 MCP servers and 14 skills discovered</code></pre>
		<pre></pre>
		<pre data-prefix="$"><code>obot skills search</code></pre>
		<pre data-prefix=">"><code
				><CheckIcon class="text-success size-4 inline" /> 56 skills available</code
			></pre>

		<pre></pre>
	</div>
{/snippet}

<svelte:head>
	<title>Obot CLI | Install</title>
</svelte:head>

<style lang="postcss">
	.mockup-code pre {
		margin-top: 0;
		margin-bottom: 0;
		border-radius: 0;
	}

	.mockup-code pre[data-prefix] {
		display: grid;
		grid-template-columns: 2rem minmax(0, 1fr);
		column-gap: 0.5rem;
	}

	.mockup-code pre[data-prefix]::before {
		grid-column: 1;
		grid-row: 1;
		align-self: start;
		display: block;
		width: 2rem;
	}

	.mockup-code pre[data-prefix] > code {
		grid-column: 2;
		grid-row: 1;
		min-width: 0;
		white-space: pre-wrap;
	}

	.inline-code {
		background-color: var(--color-base-200);
		color: var(--color-base-content);
		padding: 0.25rem 0.5rem;
		border-radius: var(--radius-md);
		font-size: var(--text-sm);

		:global(.dark) & {
			background-color: var(--color-base-300);
		}
	}

	:global(.mockup-code pre code .text-success *) {
		color: var(--color-success) !important;
	}

	:global(.command-preview pre) {
		padding-top: 0.25rem;
		padding-bottom: 0.25rem;

		&[data-prefix]::before {
			content: attr(data-prefix);
			display: inline-block;
			width: calc(0.25rem * 8);
			text-align: right;
			opacity: 50%;
			margin-right: 0.5rem;
		}
	}
</style>
