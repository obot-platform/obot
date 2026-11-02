<script lang="ts">
	import { isAbortError } from '$lib/errors';
	import { m } from '$lib/i18n';
	import type { LocalizedString } from '$lib/paraglide/messages';
	import {
		UserService,
		type MCPCatalogEntry,
		type VMCP,
		type VMCPConfigurationPolicy,
		type VMCPInstance
	} from '$lib/services';
	import type { VMcpConnectOptions } from '$lib/services/vmcps/types';
	import {
		configurationForSnapshotUpdate,
		configurationWithRevealedValues,
		vmcpComponentId,
		vmcpManifest,
		vmcpUpdateConfigurationTargets
	} from '$lib/services/vmcps/utils';
	import { errors, mcpServersAndEntries, profile, vmcpInstances } from '$lib/stores';
	import Confirm from '../Confirm.svelte';
	import ConnectVMcp from './ConnectVMcp.svelte';
	import VMcpComponentConfigurationDialog from './VMcpComponentConfigurationDialog.svelte';
	import VMcpDiffDialog from './VMcpDiffDialog.svelte';
	import VMcpSelectInstance from './VMcpSelectInstance.svelte';
	import { CircleAlert } from '@lucide/svelte';

	interface Props {
		onConfigurationNext?: (
			configuration: VMCPConfigurationPolicy[],
			forceSingleUser: boolean
		) => void | Promise<void>;
		onConfigurationDismiss?: () => void;
		fetchVMcpOnAction?: boolean;
	}

	let { onConfigurationNext, onConfigurationDismiss, fetchVMcpOnAction = false }: Props = $props();

	let selectInstanceDialog = $state<ReturnType<typeof VMcpSelectInstance>>();
	let diffDialog = $state<ReturnType<typeof VMcpDiffDialog>>();
	let configurationDialog = $state<ReturnType<typeof VMcpComponentConfigurationDialog>>();
	let connectVMcpDialog = $state<ReturnType<typeof ConnectVMcp>>();
	let showUpdateConfirm = $state(false);
	let updateName = $state('');
	let updating = $state(false);
	let onSelectInstance = $state<(instance: VMCPInstance) => void>();
	let selectInstanceTitle = $state<string>(m.vmcps_select_your_connection());
	let pendingUpdate = $state<() => Promise<void>>();
	let pendingUpdateVMcp = $state<VMCP>();
	let continueUpdateAfterClose = false;
	let fullVmcpController: AbortController | undefined;
	let pendingConfigUpdate:
		| {
				vmcp: VMCP;
				queue: ReturnType<typeof vmcpUpdateConfigurationTargets>;
				index: number;
				collected: Map<
					string,
					{ configuration: VMCPConfigurationPolicy[]; forceSingleUser: boolean }
				>;
				onConfirm: () => Promise<void>;
		  }
		| undefined;

	export function openSelectInstance(
		instances: VMCPInstance[],
		onSelect: (instance: VMCPInstance) => void,
		title: string = m.vmcps_select_your_connection()
	) {
		onSelectInstance = onSelect;
		selectInstanceTitle = title;
		selectInstanceDialog?.open(instances, title);
	}

	async function fetchFullVmcp(vmcp: VMCP): Promise<VMCP | undefined> {
		if (!fetchVMcpOnAction) return vmcp;
		fullVmcpController?.abort();
		const controller = new AbortController();
		fullVmcpController = controller;
		try {
			const full = await UserService.getVMCP(vmcp.id, {
				dontLogErrors: true,
				signal: controller.signal
			});
			if (controller.signal.aborted) return undefined;
			return full;
		} catch (err) {
			if (controller.signal.aborted || isAbortError(err)) return undefined;
			errors.append(m.vmcps_failed_to_load_vmcp());
			return undefined;
		} finally {
			if (fullVmcpController === controller) fullVmcpController = undefined;
		}
	}

	export async function openDiff(listed: VMCP) {
		const vmcp = await fetchFullVmcp(listed);
		if (vmcp) diffDialog?.open(vmcp);
	}

	export async function openConnect(
		listed: VMCP,
		instance?: VMCPInstance,
		options?: VMcpConnectOptions
	) {
		const vmcp = await fetchFullVmcp(listed);
		if (!vmcp) return;
		const targetInstance =
			instance ??
			vmcpInstances.current.items.find(
				(candidate) =>
					candidate.vmcpID === vmcp.id &&
					candidate.userID === profile.current.id &&
					!candidate.deleted
			);
		connectVMcpDialog?.open(vmcp, targetInstance, options);
	}

	export async function openReauthenticate(listed: VMCP, componentIDs: string[]) {
		const vmcp = await fetchFullVmcp(listed);
		if (!vmcp) return;
		const instance = vmcpInstances.current.items.find(
			(candidate) =>
				candidate.vmcpID === vmcp.id &&
				candidate.userID === profile.current.id &&
				!candidate.deleted
		);
		connectVMcpDialog?.openReauthenticate(vmcp, componentIDs, instance, { connectReturn: 'list' });
	}

	// Resolves once the user finishes or dismisses the component's OAuth prompt.
	export async function reauthenticateComponent(listed: VMCP, componentID: string) {
		const vmcp = await fetchFullVmcp(listed);
		if (!vmcp) return;
		const instance = vmcpInstances.current.items.find(
			(candidate) =>
				candidate.vmcpID === vmcp.id &&
				candidate.userID === profile.current.id &&
				!candidate.deleted
		);
		await new Promise<void>((resolve, reject) => {
			if (!connectVMcpDialog) {
				resolve();
				return;
			}
			connectVMcpDialog
				.reauthenticateComponent(vmcp, instance, componentID, {
					onConnected: resolve,
					onDismissed: resolve
				})
				.catch(reject);
		});
	}

	export function openEditInstanceConfiguration(
		vmcp: VMCP,
		instance: VMCPInstance,
		options?: VMcpConnectOptions
	) {
		fullVmcpController?.abort();
		const target = fetchVMcpOnAction ? vmcp.id : vmcp;
		connectVMcpDialog?.openEditConfiguration(target, instance, options).catch((err) => {
			if (!fetchVMcpOnAction) throw err;
			errors.append(m.vmcps_failed_to_load_vmcp());
		});
	}

	export function openConfiguration(
		entry: MCPCatalogEntry,
		options?: {
			configuration?: VMCPConfigurationPolicy[];
			forceSingleUser?: boolean;
			isShared?: boolean;
			submitLabel?: LocalizedString;
			errorMessage?: LocalizedString;
		}
	) {
		pendingConfigUpdate = undefined;
		continueUpdateAfterClose = false;
		configurationDialog?.open(entry, options);
	}

	export async function openUpdateConfirm(listed: VMCP, onConfirm: () => Promise<void>) {
		const vmcp = await fetchFullVmcp(listed);
		if (!vmcp) return;
		updateName = vmcp.displayName || m.vmcps_untitled_vmcp();
		pendingUpdateVMcp = vmcp;
		pendingUpdate = onConfirm;
		showUpdateConfirm = true;
	}

	async function openCurrentUpdateConfiguration() {
		const pending = pendingConfigUpdate;
		const current = pending?.queue[pending.index];
		if (!pending || !current) return;

		let configuration = current.component.configuration;
		const componentId = vmcpComponentId(current.component);
		try {
			const revealed = await UserService.revealVMCP(pending.vmcp.id, { dontLogErrors: true });
			configuration = configurationWithRevealedValues(
				configuration,
				revealed.components[componentId]
			);
		} catch {
			// Continue without revealed values when configuration is unavailable.
		}

		configurationDialog?.open(current.entry, {
			configuration,
			isShared: !pending.vmcp.userID,
			forceSingleUser: current.component.forceSingleUser,
			submitLabel: pending.index === pending.queue.length - 1 ? m.core_update() : m.core_next(),
			errorMessage: m.vmcps_failed_to_update()
		});
	}

	async function saveCollectedConfiguration() {
		const pending = pendingConfigUpdate;
		if (!pending || pending.collected.size === 0) return;

		const latest = await UserService.getVMCP(pending.vmcp.id);
		const components = (latest.components ?? []).map((component) => {
			const next = pending.collected.get(vmcpComponentId(component));
			if (!next) return component;
			return {
				...component,
				configuration: configurationForSnapshotUpdate(component, next.configuration),
				forceSingleUser: next.forceSingleUser
			};
		});
		await UserService.updateVMCP(latest.id, {
			...vmcpManifest(latest),
			components
		});
	}

	async function handleConfigurationNext(
		configuration: VMCPConfigurationPolicy[],
		forceSingleUser: boolean
	) {
		const pending = pendingConfigUpdate;
		if (!pending) {
			await onConfigurationNext?.(configuration, forceSingleUser);
			return;
		}

		const current = pending.queue[pending.index];
		if (!current) return;
		pending.collected.set(vmcpComponentId(current.component), {
			configuration,
			forceSingleUser
		});
		if (pending.index + 1 < pending.queue.length) {
			pending.index += 1;
			continueUpdateAfterClose = true;
			return;
		}

		try {
			await saveCollectedConfiguration();
			const onConfirm = pending.onConfirm;
			pendingConfigUpdate = undefined;
			await onConfirm();
		} catch {
			errors.append(m.vmcps_failed_to_update());
			throw new Error(m.vmcps_failed_to_update());
		}
	}

	function handleConfigurationClose() {
		if (continueUpdateAfterClose) {
			continueUpdateAfterClose = false;
			void openCurrentUpdateConfiguration();
			return;
		}
		pendingConfigUpdate = undefined;
		onConfigurationDismiss?.();
	}

	async function confirmUpdate() {
		const vmcp = pendingUpdateVMcp;
		const onConfirm = pendingUpdate;
		if (!vmcp) return;

		const queue = vmcpUpdateConfigurationTargets(vmcp, mcpServersAndEntries.current.entries);
		showUpdateConfirm = false;
		if (queue.length === 0) {
			await onConfirm?.();
			return;
		}

		pendingConfigUpdate = {
			vmcp,
			queue,
			index: 0,
			collected: new Map(),
			onConfirm: onConfirm ?? (async () => {})
		};
		await openCurrentUpdateConfiguration();
	}
</script>

<ConnectVMcp bind:this={connectVMcpDialog} />

<VMcpSelectInstance
	bind:this={selectInstanceDialog}
	title={selectInstanceTitle}
	onSelectInstance={(instance) => onSelectInstance?.(instance)}
/>

<VMcpDiffDialog bind:this={diffDialog} />

<VMcpComponentConfigurationDialog
	bind:this={configurationDialog}
	onNext={handleConfigurationNext}
	onClose={handleConfigurationClose}
/>

<Confirm
	show={showUpdateConfirm}
	onsuccess={async () => {
		updating = true;
		try {
			await confirmUpdate();
		} finally {
			updating = false;
		}
	}}
	oncancel={() => (showUpdateConfirm = false)}
	loading={updating}
	type="info"
	title={m.vmcps_confirm_update()}
>
	{#snippet msgContent()}
		<h4 class="flex items-center justify-center gap-2 text-lg font-semibold">
			<CircleAlert class="size-5" />
			{m.vmcps_continue_with_update()}
		</h4>
	{/snippet}
	{#snippet note()}
		<p class="text-sm font-light">
			{m.vmcps_update_confirm_note({ name: updateName })}
		</p>
	{/snippet}
</Confirm>
