import { UserService, type VMCP, type VMCPInstance } from '$lib/services';
import {
	vmcpHasUserAllowedConfiguration,
	vmcpInstanceNeedsUserConfiguration,
	vmcpNeedsUpdate
} from '$lib/services/vmcps/utils';
import { errors, profile, vmcpInstances } from '$lib/stores';
import { success } from '$lib/stores/success';
import { poll } from '$lib/utils';

export type OpenSelectInstance = (
	instances: VMCPInstance[],
	onSelect: (instance: VMCPInstance) => void,
	title?: string
) => void;

export type OpenEditInstanceConfiguration = (vmcp: VMCP, instance: VMCPInstance) => void;

export type OpenUpdateConfirm = (vmcp: VMCP, onConfirm: () => Promise<void>) => void;

export type OpenDiff = (vmcp: VMCP) => void;

export const vmcpActionProgress = $state({
	updatingId: undefined as string | undefined,
	disconnectingId: undefined as string | undefined
});

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

export function vmcpIsDisconnecting(vmcpId: string, instanceIds: string[]) {
	const id = vmcpActionProgress.disconnectingId;
	return id === vmcpId || instanceIds.some((instanceId) => instanceId === id);
}

async function disconnectInstance(instanceID: string) {
	vmcpActionProgress.disconnectingId = instanceID;
	try {
		await UserService.deleteVMCPInstance(instanceID);
		vmcpInstances.remove(instanceID);
	} catch {
		errors.append('Failed to disconnect from vMCP.');
	} finally {
		if (vmcpActionProgress.disconnectingId === instanceID) {
			vmcpActionProgress.disconnectingId = undefined;
		}
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

	vmcpActionProgress.disconnectingId = vmcp.id;
	await new Promise((resolve) => setTimeout(resolve, 1000));
	if (vmcpActionProgress.disconnectingId === vmcp.id) {
		vmcpActionProgress.disconnectingId = undefined;
	}
	toggle(false);
}

export async function updateVMcp(
	vmcp: VMCP,
	onUpdated?: (vmcp: VMCP) => void,
	isCancelled?: () => boolean
) {
	const name = vmcp.displayName || 'Untitled vMCP';
	vmcpActionProgress.updatingId = vmcp.id;
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
		if (vmcpActionProgress.updatingId === vmcp.id) {
			vmcpActionProgress.updatingId = undefined;
		}
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
