<script lang="ts" module>
	const SEMVER_PATTERN =
		/^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$/;
	const DEVELOPMENT_VERSION_PATTERN = /^v?0\.0\.0-dev(?:[-.+]|$)/;

	function dockerTagForVersion(rawVersion?: string): string {
		const normalizedVersion = rawVersion?.trim();
		if (
			!normalizedVersion ||
			!SEMVER_PATTERN.test(normalizedVersion) ||
			DEVELOPMENT_VERSION_PATTERN.test(normalizedVersion) ||
			normalizedVersion.endsWith('-dirty')
		) {
			return 'main';
		}

		return normalizedVersion.split('+', 1)[0];
	}
</script>

<script lang="ts">
	import CopyButton from '$lib/components/CopyButton.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import type { MCPTunnel } from '$lib/services';
	import { version } from '$lib/stores';
	import { Container, KeyRound, Terminal, TriangleAlert } from '@lucide/svelte';

	type CommandTab = 'docker' | 'cli';

	interface Props {
		action?: 'created' | 'rotated';
		onClose: () => void;
		tunnel?: MCPTunnel;
	}

	let { action = 'created', onClose, tunnel }: Props = $props();
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();
	let commandTab = $state<CommandTab>('docker');
	let obotURL = $derived(
		typeof window === 'undefined' ? '<OBOT_URL>/api' : `${window.location.origin}/api`
	);
	let cliCommand = $derived(
		tunnel ? `obot tunnel --obot-base-url ${obotURL} --token ${tunnel.token}` : ''
	);
	let dockerTag = $derived(dockerTagForVersion(version.current.obot));
	let dockerCommand = $derived(
		tunnel
			? `docker run --rm ghcr.io/obot-platform/obot:${dockerTag} tunnel --obot-base-url ${obotURL} --token ${tunnel.token}`
			: ''
	);
	let command = $derived(commandTab === 'docker' ? dockerCommand : cliCommand);

	$effect(() => {
		if (tunnel?.token) {
			commandTab = 'docker';
			dialog?.open();
		}
	});
</script>

{#if tunnel?.token}
	<ResponsiveDialog
		bind:this={dialog}
		{onClose}
		title={action === 'created' ? 'MCP Tunnel Created' : 'MCP Tunnel Secret Rotated'}
		class="w-full max-w-2xl"
		disableClickOutside
	>
		<div class="flex flex-col gap-6">
			<div class="notification-alert">
				<div class="flex items-start gap-3">
					<TriangleAlert class="size-5 shrink-0" />
					<div class="flex flex-col gap-1">
						<p class="text-sm font-medium">Save this secret now</p>
						<p class="text-xs">
							This is the only time the complete tunnel secret will be shown. Store it securely
							before closing this dialog.
						</p>
					</div>
				</div>
			</div>

			<div class="flex flex-col gap-2">
				<p class="text-sm font-medium">Tunnel Secret</p>
				<div class="flex items-center gap-2">
					<div
						class="bg-base-200 flex min-w-0 flex-1 items-center gap-2 rounded-md border px-3 py-2"
					>
						<KeyRound class="text-muted-content size-4 shrink-0" />
						<code class="flex-1 font-mono text-sm break-all">{tunnel.token}</code>
					</div>
					<CopyButton text={tunnel.token} buttonText="Copy" />
				</div>
			</div>

			<div class="flex flex-col gap-2">
				<p class="text-sm font-medium">Connect this tunnel</p>
				<div class="tabs tabs-box w-fit" role="tablist" aria-label="Tunnel command type">
					<button
						type="button"
						role="tab"
						aria-selected={commandTab === 'docker'}
						aria-controls="tunnel-command-panel"
						class="tab gap-2"
						class:tab-active={commandTab === 'docker'}
						onclick={() => (commandTab = 'docker')}
					>
						<Container class="size-4" />
						Docker
					</button>
					<button
						type="button"
						role="tab"
						aria-selected={commandTab === 'cli'}
						aria-controls="tunnel-command-panel"
						class="tab gap-2"
						class:tab-active={commandTab === 'cli'}
						onclick={() => (commandTab = 'cli')}
					>
						<Terminal class="size-4" />
						CLI
					</button>
				</div>
				<div id="tunnel-command-panel" role="tabpanel" class="flex items-start gap-2">
					<div
						class="bg-base-200 flex min-w-0 flex-1 items-start gap-2 rounded-md border px-3 py-2"
					>
						{#if commandTab === 'docker'}
							<Container class="text-muted-content mt-0.5 size-4 shrink-0" />
						{:else}
							<Terminal class="text-muted-content mt-0.5 size-4 shrink-0" />
						{/if}
						<code class="flex-1 font-mono text-sm break-all">{command}</code>
					</div>
					<CopyButton text={command} buttonText="Copy" />
				</div>
			</div>
		</div>

		<div class="mt-6 flex justify-end">
			<button class="btn btn-primary" onclick={() => dialog?.close()}>
				I've saved the secret
			</button>
		</div>
	</ResponsiveDialog>
{/if}
