<script lang="ts">
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
	}

	let { onConfigurationNext }: Props = $props();

	let selectInstanceDialog = $state<ReturnType<typeof VMcpSelectInstance>>();
	let diffDialog = $state<ReturnType<typeof VMcpDiffDialog>>();
	let configurationDialog = $state<ReturnType<typeof VMcpComponentConfigurationDialog>>();
	let connectVMcpDialog = $state<ReturnType<typeof ConnectVMcp>>();
	let showUpdateConfirm = $state(false);
	let updateName = $state('');
	let updating = $state(false);
	let onSelectInstance = $state<(instance: VMCPInstance) => void>();
	let selectInstanceTitle = $state('Select Your Connection');
	let pendingUpdate = $state<() => Promise<void>>();
	let pendingUpdateVMcp = $state<VMCP>();
	let continueUpdateAfterClose = false;
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
		title = 'Select Your Connection'
	) {
		onSelectInstance = onSelect;
		selectInstanceTitle = title;
		selectInstanceDialog?.open(instances, title);
	}

	export function openDiff(vmcp: VMCP) {
		diffDialog?.open(vmcp);
	}

	export function openConnect(vmcp: VMCP, instance?: VMCPInstance, options?: VMcpConnectOptions) {
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

	export function openEditInstanceConfiguration(
		vmcp: VMCP,
		instance: VMCPInstance,
		options?: VMcpConnectOptions
	) {
		void connectVMcpDialog?.openEditConfiguration(vmcp, instance, options);
	}

	export function openConfiguration(
		entry: MCPCatalogEntry,
		options?: {
			configuration?: VMCPConfigurationPolicy[];
			forceSingleUser?: boolean;
			submitLabel?: string;
			errorMessage?: string;
		}
	) {
		pendingConfigUpdate = undefined;
		continueUpdateAfterClose = false;
		configurationDialog?.open(entry, options);
	}

	export function openUpdateConfirm(vmcp: VMCP, onConfirm: () => Promise<void>) {
		updateName = vmcp.displayName || 'Untitled vMCP';
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
			forceSingleUser: current.component.forceSingleUser,
			submitLabel: pending.index === pending.queue.length - 1 ? 'Update' : 'Next',
			errorMessage: 'Failed to update vMCP.'
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
			errors.append('Failed to update vMCP.');
			throw new Error('Failed to update vMCP.');
		}
	}

	function handleConfigurationClose() {
		if (continueUpdateAfterClose) {
			continueUpdateAfterClose = false;
			void openCurrentUpdateConfiguration();
			return;
		}
		pendingConfigUpdate = undefined;
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
	title="Confirm Update"
>
	{#snippet msgContent()}
		<h4 class="flex items-center justify-center gap-2 text-lg font-semibold">
			<CircleAlert class="size-5" />
			{`Update ${updateName}?`}
		</h4>
	{/snippet}
	{#snippet note()}
		<p class="text-sm font-light">The vMCP and its components will be updated to latest version.</p>
	{/snippet}
</Confirm>
