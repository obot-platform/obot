<script lang="ts">
	import { clickOutside } from '$lib/actions/clickoutside';
	import { formatNumber } from '$lib/format';
	import type { MCPManifest, ProjectMCP } from '$lib/services';
	import { darkMode, responsive } from '$lib/stores';
	import { ChevronRight, ChevronsRight, Server, Star, X } from 'lucide-svelte';
	import { fade } from 'svelte/transition';
	import { isValidMcpConfig, type MCPServerInfo } from '$lib/services/chat/mcp';
	import HostedMcpForm from '$lib/components/mcp/HostedMcpForm.svelte';

	interface Props {
		manifest?: MCPManifest | ProjectMCP;
		disableOutsideClick?: boolean;
		hideCloseButton?: boolean;
		onUpdate?: (manifest: MCPServerInfo) => void;
		onConfigure?: () => void;
		selected?: boolean;
		submitText?: string;
		configureText?: string;
	}
	let {
		manifest,
		disableOutsideClick,
		hideCloseButton,
		onUpdate,
		onConfigure,
		selected,
		submitText,
		configureText
	}: Props = $props();
	let dialog: HTMLDialogElement | undefined = $state();
	let config = $state(initFromManifest(manifest));
	let showConfig = $state(false);
	let showSubmitError = $state(false);

	function initFromManifest(manifest?: MCPManifest | ProjectMCP) {
		if (manifest && 'server' in manifest) {
			return {
				...manifest.server,
				env: manifest.server.env?.map((e) => ({ ...e, value: '', custom: false })) ?? [],
				args: manifest.server.args ? [...manifest.server.args] : [],
				command: manifest.server.command ?? '',
				headers: manifest.server.headers?.map((e) => ({ ...e, value: '', custom: false })) ?? []
			};
		}

		return {
			...manifest,
			name: manifest?.name ?? '',
			description: manifest?.description ?? '',
			icon: manifest?.icon ?? '',
			env: manifest?.env?.map((e) => ({ ...e, value: '', custom: false })) ?? [],
			args: manifest?.args ? [...manifest.args] : [],
			command: manifest?.command ?? '',
			headers: manifest?.headers?.map((e) => ({ ...e, value: '', custom: false })) ?? []
		};
	}

	$effect(() => {
		if (manifest) {
			config = initFromManifest(manifest);
		}
	});

	export function open() {
		dialog?.showModal();
	}

	function reset() {
		showConfig = false;
		showSubmitError = false;
		config = initFromManifest(manifest);
	}

	export function close() {
		dialog?.close();
		reset();
	}

	function handleSubmit() {
		if (!manifest) return;
		if (!isValidMcpConfig(config)) {
			showSubmitError = true;
			return;
		}

		if ('server' in manifest) {
			onUpdate?.({
				...manifest.server,
				...config
			});
		} else {
			onUpdate?.({
				...manifest,
				...config
			});
		}
		dialog?.close();
		reset();
	}
</script>

<dialog
	bind:this={dialog}
	class="default-dialog w-full sm:max-w-lg"
	class:mobile-screen-dialog={responsive.isMobile}
	use:clickOutside={() => {
		if (disableOutsideClick) return;
		close();
	}}
>
	<div class="grid h-fit max-h-[calc(100vh-4rem)] grid-rows-[auto_1fr_auto]">
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
			</div>
			<div class="default-scrollbar-thin min-h-0 w-full overflow-y-auto px-4 py-1 md:px-6">
				{#if showConfig}
					<div class="flex w-full flex-col gap-4" in:fade>
						<HostedMcpForm bind:config {showSubmitError} />
					</div>
				{:else}
					{@render readOnlyView()}
				{/if}
			</div>
			<div class="flex justify-end px-4 py-4 md:px-6">
				{#if showConfig}
					<button
						onclick={handleSubmit}
						class="button-primary flex w-full items-center justify-center gap-1 self-end md:w-fit"
					>
						{selected ? 'Update' : (submitText ?? 'Add to Agent')}
						<ChevronsRight class="size-4" />
					</button>
				{:else}
					<button
						onclick={() => {
							if (onConfigure) {
								onConfigure();
							} else {
								showConfig = true;
							}
						}}
						class="button-primary flex w-full items-center justify-center gap-1 self-end md:w-fit"
					>
						{configureText ?? 'Configure'}
						<ChevronsRight class="size-4" />
					</button>
				{/if}
			</div>
		{/if}
	</div>
</dialog>

{#snippet readOnlyView()}
	{#if manifest && 'server' in manifest && manifest.server.env?.some((env) => env.required)}
		<div
			class="border-surface2 dark:border-surface3 relative mt-2 w-full rounded-lg border-2 p-5 pt-2"
		>
			<h4
				class="dark:bg-surface2 absolute top-0 left-3 w-fit -translate-y-3.5 bg-white px-2 text-base font-semibold"
			>
				What You'll Need
			</h4>
			<ul class="mt-4 flex flex-col items-baseline gap-4">
				{#each manifest.server.env.filter((env) => env.required) as env}
					<li class="flex w-full flex-col">
						<div class="text-sm font-semibold capitalize">{env.name}</div>
						<div class="text-xs font-light text-gray-500">{env.description}</div>
					</li>
				{/each}
			</ul>
		</div>
		<!-- display tools part of the mcp server here once it's implemented-->
	{/if}
{/snippet}
