<script lang="ts">
	import { MDM_DEVICES_CONFIGURATION_FIELD_IDS } from '$lib/constants';
	import { parseErrorContent } from '$lib/errors';
	import Loading from '$lib/icons/Loading.svelte';
	import { AdminService, type MDMConfiguration } from '$lib/services';
	import { MonitorSmartphone, TriangleAlert } from '@lucide/svelte';

	interface Props {
		readOnly?: boolean;
		onCreate: (result: MDMConfiguration) => void;
	}

	let { readOnly = false, onCreate }: Props = $props();
	let loading = $state(false);
	let createError = $state<string>();
	let enforcementEnabled = $state(false);

	async function handleCreate() {
		loading = true;
		createError = undefined;
		try {
			// The allowlist is deliberately omitted: the server seeds its own default
			// policy when a configuration is created with enforcement enabled.
			const created = await AdminService.createMDMConfiguration(
				enforcementEnabled ? { enforcementEnabled: true } : {}
			);
			onCreate(created);
		} catch (error) {
			createError = parseErrorContent(error).message;
		} finally {
			loading = false;
		}
	}
</script>

<div
	class="mx-auto flex h-full w-full max-w-2xl flex-col items-center justify-start pt-[12vh] pb-12"
>
	<div class="paper flex w-full flex-col items-center gap-6 p-8 text-center">
		<div class="bg-primary/10 flex size-20 items-center justify-center rounded-full">
			<MonitorSmartphone class="text-primary size-10" />
		</div>

		<div class="flex max-w-lg flex-col gap-2">
			<h3 class="text-xl font-semibold">Configure Managed Devices</h3>
			<p class="text-muted-content text-sm">
				Enable user device scanning, local agent audit logs, and more
			</p>
		</div>

		{#if createError}
			<div class="notification-alert flex w-full items-start gap-2 p-3 text-left text-sm">
				<TriangleAlert class="size-5 shrink-0 text-warning" />
				<span class="break-all">{createError}</span>
			</div>
		{/if}

		{#if readOnly}
			<p class="text-muted-content text-sm">
				An administrator with write access must create the initial configuration.
			</p>
		{:else}
			<label class="flex w-full items-start gap-3 text-left text-sm">
				<input type="checkbox" class="mt-0.5" bind:checked={enforcementEnabled} />
				<span class="flex flex-col gap-0.5">
					<span class="flex flex-wrap items-center gap-1.5 font-medium">
						Enforce tool calls on enrolled devices
						<span class="badge badge-warning badge-sm">Experimental</span>
					</span>
					<span class="input-description">
						Blocks tool calls that aren't on the allowlist. Starts with Obot-hosted MCP servers and
						built-in agent tools allowed. Requires installing the Obot Sentry package on your
						devices. This feature is experimental and is not recommended for production use — a
						misconfigured allowlist blocks real work on every enrolled device.
					</span>
				</span>
			</label>

			<button
				id={MDM_DEVICES_CONFIGURATION_FIELD_IDS.getStartedButton}
				class="btn btn-primary flex items-center gap-2"
				disabled={loading}
				onclick={handleCreate}
			>
				{#if loading}<Loading class="size-4" />{/if}
				Get Started
			</button>
		{/if}
	</div>
</div>
