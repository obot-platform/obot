import { UserService, type VMCP, type VMCPInstance } from '$lib/services';
import {
	vmcpHasUserAllowedConfiguration,
	vmcpInstanceNeedsUserConfiguration,
	vmcpNeedsUpdate
} from '$lib/services/vmcps/utils';
import { errors, profile, vmcpInstances } from '$lib/stores';
import { success } from '$lib/stores/success';
import { poll } from '$lib/utils';
import { SvelteSet } from 'svelte/reactivity';

export type OpenSelectInstance = (
	instances: VMCPInstance[],
	onSelect: (instance: VMCPInstance) => void,
	title?: string
) => void;

export type OpenEditInstanceConfiguration = (vmcp: VMCP, instance: VMCPInstance) => void;

export type OpenUpdateConfirm = (vmcp: VMCP, onConfirm: () => Promise<void>) => void;

export type OpenDiff = (vmcp: VMCP) => void;

export const vmcpActionProgress = {
	updatingIds: new SvelteSet<string>(),
	disconnectingIds: new SvelteSet<string>()
};

function startAction(ids: SvelteSet<string>, id: string) {
	if (ids.has(id)) return false;
	ids.add(id);
	return true;
}

export function vmcpItemContext(vmcp: VMCP) {
	const isCreator = Boolean(vmcp.userID && profile.current.id === vmcp.userID);
	const canDelete = Boolean(profile.current.isAdmin?.() || isCreator);
	const myInstances = vmcpInstances.current.items.filter(
		(instance) =>
			instance.vmcpID === vmcp.id && instance.userID === profile.current.id && !instance.deleted
	);
	const instancesNeedingConfiguration = myInstances.filter((instance) =>
		vmcpInstanceNeedsUserConfiguration(instance)
	);

	return {
		isCreator,
		canDelete,
		canUpdate: canDelete,
		canConnect: !vmcp.userID || isCreator,
		needsUpdate: vmcpNeedsUpdate(vmcp),
		myInstances,
		connected: myInstances.length > 0,
		instancesNeedingConfiguration,
		canEditInstanceConfiguration: Boolean(
			vmcpHasUserAllowedConfiguration(vmcp) && myInstances.length > 0
		),
		hasActions: isCreator || Boolean(profile.current.hasAdminAccess?.()) || myInstances.length > 0,
		name: vmcp.displayName || 'Untitled vMCP'
	};
}

export function vmcpIsUpdating(vmcpId: string) {
	return vmcpActionProgress.updatingIds.has(vmcpId);
}

export function vmcpIsDisconnecting(vmcpId: string, instanceIds: string[]) {
	const ids = vmcpActionProgress.disconnectingIds;
	return ids.has(vmcpId) || instanceIds.some((instanceId) => ids.has(instanceId));
}

async function disconnectInstance(instanceID: string) {
	if (!startAction(vmcpActionProgress.disconnectingIds, instanceID)) return;
	try {
		await UserService.deleteVMCPInstance(instanceID);
		vmcpInstances.remove(instanceID);
	} catch {
		errors.append('Failed to disconnect from vMCP.');
	} finally {
		vmcpActionProgress.disconnectingIds.delete(instanceID);
	}
}

export async function resetVMcpConnection(
	vmcp: VMCP,
	toggle: (open?: boolean) => void,
	openSelectInstance?: OpenSelectInstance
) {
	const { myInstances, connected } = vmcpItemContext(vmcp);
	if (openSelectInstance && connected && myInstances.length > 0) {
		if (myInstances.length === 1) {
			await disconnectInstance(myInstances[0].id);
			toggle(false);
			return;
		}
		openSelectInstance(
			myInstances,
			(instance) => {
				void disconnectInstance(instance.id);
			},
			'Select Connection to Disconnect'
		);
		toggle(false);
		return;
	}

	if (!startAction(vmcpActionProgress.disconnectingIds, vmcp.id)) {
		toggle(false);
		return;
	}
	try {
		await new Promise((resolve) => setTimeout(resolve, 1000));
	} finally {
		vmcpActionProgress.disconnectingIds.delete(vmcp.id);
		toggle(false);
	}
}

export async function updateVMcp(
	vmcp: VMCP,
	onUpdated?: (vmcp: VMCP) => void,
	isCancelled?: () => boolean
) {
	if (!startAction(vmcpActionProgress.updatingIds, vmcp.id)) return;
	const name = vmcp.displayName || 'Untitled vMCP';
	try {
		await UserService.triggerVMCPUpdate(vmcp.id);
		let updated: VMCP | undefined;
		await poll(
			async () => {
				if (isCancelled?.()) return true;
				updated = await UserService.getVMCP(vmcp.id);
				return Boolean(isCancelled?.()) || !vmcpNeedsUpdate(updated);
			},
			{ interval: 1000 }
		);
		if (isCancelled?.() || !updated || vmcpNeedsUpdate(updated)) return;
		onUpdated?.(updated);
		success.add(`Updated ${name}.`);
	} catch {
		if (!isCancelled?.()) {
			errors.append('Failed to update vMCP.');
		}
	} finally {
		vmcpActionProgress.updatingIds.delete(vmcp.id);
	}
}

export function editVMcpInstanceConfiguration(
	vmcp: VMCP,
	openSelectInstance: OpenSelectInstance | undefined,
	openEditInstanceConfiguration: OpenEditInstanceConfiguration | undefined,
	toggle?: (open?: boolean) => void
) {
	if (!openEditInstanceConfiguration) return;
	const { myInstances } = vmcpItemContext(vmcp);
	if (myInstances.length === 0) return;
	if (myInstances.length === 1 || !openSelectInstance) {
		openEditInstanceConfiguration(vmcp, myInstances[0]);
		toggle?.(false);
		return;
	}
	openSelectInstance(
		myInstances,
		(instance) => openEditInstanceConfiguration(vmcp, instance),
		'Select Connection to Configure'
	);
	toggle?.(false);
}
