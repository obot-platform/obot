<script lang="ts">
	import McpCompositeOauth from '$lib/components/mcp/McpCompositeOauth.svelte';
	import ConnectVMcp from '$lib/components/vmcps/ConnectVMcp.svelte';
	import { isAbortError } from '$lib/errors';
	import { m } from '$lib/i18n';
	import { UserService } from '$lib/services';
	import { untrack } from 'svelte';

	let { data } = $props();
	let connection = $state<ReturnType<typeof ConnectVMcp>>();
	// Instances finish setup first; tracking the ready ID lets navigation start over without a stale view.
	let readyId = $state<string>();

	function needsInstanceSetup(id: string) {
		return id.startsWith('vmcpi1') && readyId !== id;
	}

	let error = $state('');
	let loading = $state(false);
	let controller: AbortController | undefined;

	async function configure(forId?: string) {
		const current = controller;
		const id = forId ?? data.compositeMcpId;
		if (!current || !connection) return;
		loading = true;
		error = '';
		try {
			const instance = await UserService.getVMCPInstance(id, {
				signal: current.signal
			});
			if (current.signal.aborted) return;

			await connection.openEditConfiguration(instance.vmcpID, instance, {
				onConnected: () => {
					if (!current.signal.aborted) readyId = id;
				},
				// Closing the dialog keeps a settled configuration; an unconfigured instance cannot authenticate yet.
				onDismissed: () => {
					if (current.signal.aborted) return;
					if (instance.status?.configured) {
						readyId = id;
					} else {
						error = m.vmcps_configuration_required_to_authenticate();
					}
				}
			});
		} catch (err) {
			if (!current.signal.aborted && !isAbortError(err)) {
				error = err instanceof Error ? err.message : String(err);
			}
		} finally {
			if (controller === current) loading = false;
		}
	}

	// Each connection starts over, so client-side navigation never reuses another instance's state.
	$effect(() => {
		const id = data.compositeMcpId;
		const current = new AbortController();
		controller = current;
		readyId = undefined;
		untrack(() => {
			error = '';
			loading = false;
			if (id.startsWith('vmcpi1')) void configure(id);
		});
		return () => current.abort();
	});
</script>

<!-- Remount so an open setup dialog never carries over to another connection. -->
{#key data.compositeMcpId}
	<ConnectVMcp bind:this={connection} configureOnly />
{/key}

{#if needsInstanceSetup(data.compositeMcpId)}
	<div class="colors-background flex min-h-screen flex-col items-center justify-center gap-4 p-4">
		{#if error}<p class="notification-error" role="alert">{error}</p>{/if}
		<button class="btn btn-primary" disabled={loading} onclick={() => configure()}>
			{m.mcps_retry()}
		</button>
	</div>
{:else}
	{#key data.compositeMcpId}
		<McpCompositeOauth
			compositeMcpId={data.compositeMcpId}
			oauthAuthRequestId={data.oauthAuthRequestId}
		/>
	{/key}
{/if}

<svelte:head>
	<title>Obot | {m.vmcps_authenticate()}</title>
</svelte:head>
