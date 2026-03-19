<script lang="ts">
	import { ChatService, NanobotService } from '$lib/services';
	import type { MCPCatalogServer } from '$lib/services/chat/types';
	import { errors, profile } from '$lib/stores';
	import Confirm from '$lib/components/Confirm.svelte';
	import { AlertTriangle, LoaderCircle } from 'lucide-svelte';

	interface Props {
		agentId: string;
		projectId: string;
	}

	let { agentId, projectId }: Props = $props();

	let showRestartAgentConfirm = $state(false);
	let restartingAgent = $state(false);
	let dismissed = $state(false);
	let agentServer = $state<MCPCatalogServer | null>(null);
	let loadingAgentServer = $state(false);
	let lastDismissResetKey = $state('');

	let showBanner = $derived(
		!dismissed &&
			!!agentServer &&
			agentServer.userID === profile.current.id &&
			(agentServer.needsUpdate || agentServer.needsK8sUpdate)
	);

	let bannerMessage = $derived.by(() => {
		if (agentServer?.needsUpdate && agentServer?.needsK8sUpdate) {
			return 'A pending agent update and configuration change are ready. Restart to apply both.';
		}
		if (agentServer?.needsUpdate) {
			return 'A pending agent update is ready. Restart to apply it.';
		}
		if (agentServer?.needsK8sUpdate) {
			return 'A pending agent configuration change is ready. Restart to apply it.';
		}
		return '';
	});

	$effect(() => {
		const resetKey = `${agentId}:${projectId}`;
		if (lastDismissResetKey !== resetKey) {
			dismissed = false;
			lastDismissResetKey = resetKey;
		}
	});

	$effect(() => {
		loadingAgentServer = true;
		ChatService.getMcpCatalogServer(`ms1${agentId}`)
			.then((server) => {
				agentServer = server;
			})
			.catch((error) => {
				agentServer = null;
				console.error('Failed to load agent server:', error);
			})
			.finally(() => {
				loadingAgentServer = false;
			});
	});

	async function handleRestartAgent() {
		if (!agentServer || agentServer.userID !== profile.current.id) return;
		restartingAgent = true;
		try {
			await ChatService.restartMcpServer(agentServer.id);
			await NanobotService.launchProjectV2Agent(projectId, agentId);
			agentServer = { ...agentServer, needsUpdate: false, needsK8sUpdate: false };
			window.location.reload();
		} catch (error) {
			console.error('Failed to restart agent:', error);
			errors.append(error);
		} finally {
			restartingAgent = false;
			showRestartAgentConfirm = false;
		}
	}
</script>

{#if showBanner}
	<div
		class="border-warning/40 bg-warning/10 text-warning-content flex items-start gap-3 rounded-xl border px-4 py-3"
	>
		<AlertTriangle class="text-warning mt-0.5 size-5 shrink-0" />
		<div class="min-w-0 flex-1">
			<p class="text-sm font-medium">Agent restart recommended</p>
			<p class="text-sm/5 opacity-80">{bannerMessage}</p>
		</div>
		<div class="flex shrink-0 items-center gap-2">
			<button
				type="button"
				class="btn btn-warning btn-sm"
				onclick={() => {
					showRestartAgentConfirm = true;
				}}
			>
				{#if restartingAgent || loadingAgentServer}
					<LoaderCircle class="size-4 animate-spin" />
				{/if}
				Restart agent
			</button>
			<button
				type="button"
				class="btn btn-ghost btn-sm"
				onclick={() => {
					dismissed = true;
				}}
			>
				Dismiss
			</button>
		</div>
	</div>
{/if}

<Confirm
	show={showRestartAgentConfirm}
	title="Restart Agent"
	msg="Restart this agent?"
	note="This will temporarily interrupt any active agent sessions."
	onsuccess={handleRestartAgent}
	oncancel={() => {
		showRestartAgentConfirm = false;
	}}
	loading={restartingAgent}
	type="info"
/>
