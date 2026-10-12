import { m } from '$lib/i18n';
import { UserService, type VMCP, type VMCPInstance } from '$lib/services';
import {
	isCatalogSyncedVMcp,
	vmcpHasUserAllowedConfiguration,
	vmcpInstanceNeedsUserConfiguration,
	vmcpNeedsAdminConfiguration,
	vmcpNeedsUpdate
} from '$lib/services/vmcps/utils';
import { errors, profile, vmcpInstances } from '$lib/stores';
import { success } from '$lib/stores/success';
import { hasAccessWithinSubjects } from '$lib/subjectResolver';
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
	const canUpdate = Boolean(profile.current.isAdmin?.() || isCreator);
	const canDelete = canUpdate && !isCatalogSyncedVMcp(vmcp);
	const canConnect =
		(vmcp.components?.length ?? 0) > 0 &&
		(!vmcp.userID
			? hasAccessWithinSubjects(vmcp.profiles?.flatMap((p) => p.subjects) ?? [], profile.current)
			: isCreator);

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
		canUpdate,
		canConnect,
		needsUpdate: vmcpNeedsUpdate(vmcp),
		needsAdminConfiguration: vmcpNeedsAdminConfiguration(vmcp),
		myInstances,
		connected: myInstances.length > 0,
		instancesNeedingConfiguration,
		canEditInstanceConfiguration: Boolean(
			vmcpHasUserAllowedConfiguration(vmcp) && myInstances.length > 0
		),
		hasActions: isCreator || Boolean(profile.current.hasAdminAccess?.()) || myInstances.length > 0,
		name: vmcp.displayName || m.vmcps_untitled_vmcp(),
		isShared: !vmcp.userID
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
		errors.append(m.vmcps_failed_to_disconnect());
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
			m.vmcps_select_connection_to_disconnect()
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
	const name = vmcp.displayName || m.vmcps_untitled_vmcp();
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
		success.add(m.vmcps_updated_named({ name }));
	} catch {
		if (!isCancelled?.()) {
			errors.append(m.vmcps_failed_to_update());
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
		m.vmcps_select_connection_to_configure()
	);
	toggle?.(false);
}
