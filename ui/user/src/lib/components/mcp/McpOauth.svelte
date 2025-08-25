<script lang="ts">
	import {
		AdminService,
		ChatService,
		type MCPCatalogEntry,
		type MCPCatalogServer,
		type Project,
		type ProjectMCP
	} from '$lib/services';
	import { parseErrorContent } from '$lib/errors';
	import { Info, LoaderCircle, RefreshCcw } from 'lucide-svelte';
	import { onMount } from 'svelte';
	import { AllSelection } from '@milkdown/prose/state';

	interface Props {
		entry: MCPCatalogEntry | MCPCatalogServer | ProjectMCP;
		onAuthenticate?: () => void;
		error?: string;
		project?: Project;
		text?: string;
	}

	let { onAuthenticate, error = $bindable(), project, entry, text }: Props = $props();

	let oauthURL = $state<string>('');
	let showRefresh = $state(false);
	let loading = $state(false);
	// Create AbortController for cancelling API calls
	let abortController = $state<AbortController | null>(null);
	let initializedListener = $state(false);

	const handleVisibilityChange = () => {
		if (!showRefresh || loading) return;
		if (document.visibilityState === 'visible') {
			loadOauthURL();
		}
	};

	async function loadOauthURL() {
		// Cancel any existing requests
		if (abortController) {
			abortController.abort();
		}

		// Create new AbortController for this request
		abortController = new AbortController();

		loading = true;
		oauthURL = '';
		error = '';

		try {
			if (project) {
				oauthURL = await ChatService.getProjectMcpServerOauthURL(
					project.assistantID,
					project.id,
					entry.id,
					{
						signal: abortController.signal
					}
				);
			} else if ('sharedWithinCatalogName' in entry) {
				oauthURL = await AdminService.getMCPCatalogServerOAuthURL(
					entry.sharedWithinCatalogName,
					entry.id,
					{
						signal: abortController.signal
					}
				);
			} else {
				oauthURL = await ChatService.getMcpServerOauthURL(entry.id, {
					signal: abortController.signal
				});
			}
		} catch (err: unknown) {
			// Only handle errors if the request wasn't aborted
			if (err instanceof Error && err.name !== 'AbortError') {
				console.error(err);
				const { message } = parseErrorContent(err);
				error = message;
			}
		} finally {
			loading = false;

			if (!oauthURL && showRefresh) {
				onAuthenticate?.();
				showRefresh = false;
			}

			if (oauthURL && !initializedListener) {
				document.addEventListener('visibilitychange', handleVisibilityChange);
			} else if (!oauthURL) {
				document.removeEventListener('visibilitychange', handleVisibilityChange);
			}
		}
	}

	onMount(() => {
		loadOauthURL();

		return () => {
			document.removeEventListener('visibilitychange', handleVisibilityChange);
		};
	});
</script>

{#if oauthURL}
	<div class="notification-info flex w-full flex-row justify-between p-3 text-sm font-light">
		<div class="flex items-center gap-3">
			<Info class="size-6 flex-shrink-0" />
			{#if text}
				<p>{text}</p>
			{:else}
				<p>For detailed information about this MCP server, server authentication is required.</p>
			{/if}
		</div>
		{#if showRefresh && loading}
			<div class="flex items-center gap-2 text-sm font-light">
				<LoaderCircle class="size-4 animate-spin" />
				Authenticating...
			</div>
		{:else}
			<a
				target="_blank"
				href={oauthURL}
				class="button-primary text-center text-sm"
				onclick={() => {
					setTimeout(() => {
						showRefresh = true;
					}, 500);
				}}
			>
				Authenticate
			</a>
		{/if}
	</div>
{/if}
