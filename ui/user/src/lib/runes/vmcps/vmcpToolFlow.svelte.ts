import {
	UserService,
	type CompositeServerToolRow,
	type MCPCatalogEntry,
	type ToolOverride,
	type VMCP,
	type VMCPComponent,
	type VMCPManifest
} from '$lib/services';
import { compositeEffectiveToolNames, toolOverridesFromRows } from '$lib/services/user/mcp';
import { errors } from '$lib/stores';
import { success } from '$lib/stores/success';

export type VMcpToolDialog = 'added-confirm' | 'added-create' | 'setup' | 'edit' | 'actions';

interface PendingAddedServer {
	component: VMCPComponent;
	vmcp: VMCP;
}

interface PendingRemoval {
	component: {
		id?: string;
		name?: string;
		description?: string;
		icon?: string;
	};
	vmcp: VMCP;
}

interface VMcpToolFlowOptions {
	onUpdated?: (vmcp: VMCP) => void | Promise<void>;
}

function componentId(component: Pick<VMCPComponent, 'id' | 'mcpServerCatalogEntryID'>) {
	return component.id || component.mcpServerCatalogEntryID || '';
}

function componentEntry(component: VMCPComponent): MCPCatalogEntry {
	return {
		id: componentId(component),
		created: new Date(0).toISOString(),
		manifest: component.catalogEntry.manifest,
		isCatalogEntry: true,
		type: 'catalog-entry',
		unsupportedTools: component.catalogEntry.unsupportedTools
	};
}

function toolRowsFromComponent(component: VMCPComponent): CompositeServerToolRow[] {
	const id = componentId(component);
	const preview = component.catalogEntry.manifest.toolPreview ?? [];
	const previewByName = new Map(preview.map((tool) => [tool.name, tool]));
	const overrides = component.toolOverrides ?? [];
	const sourceTools: ToolOverride[] =
		overrides.length > 0 ? overrides : preview.map((tool) => ({ name: tool.name }));

	return sourceTools.map((override) => {
		const previewTool = previewByName.get(override.name);
		const description = override.description ?? previewTool?.description;
		return {
			id: `${id}-${override.name}`,
			name: override.name,
			overrideName: (override.overrideName || '').trim() || override.name,
			description,
			overrideDescription: (override.overrideDescription || '').trim() || description,
			enabled: overrides.length === 0 || override.enabled === true
		};
	});
}

function vmcpManifest(vmcp: VMCP, components: VMCPComponent[]): VMCPManifest {
	return {
		displayName: vmcp.displayName,
		description: vmcp.description,
		icon: vmcp.icon,
		components,
		profiles: vmcp.profiles,
		forceSingleUser: vmcp.forceSingleUser
	};
}

/**
 * Owns the state machine for choosing, editing, refreshing, and removing tools on a vMCP.
 * Dialog rendering is kept in VMcpToolDialogs; this module owns transitions and persistence.
 */
export function createVMcpToolFlow(options: VMcpToolFlowOptions = {}) {
	let dialog = $state<VMcpToolDialog>();
	let addedServer = $state<PendingAddedServer>();
	let pendingRemoval = $state<PendingRemoval>();
	let removing = $state(false);
	let modifyingVMcp = $state<VMCP>();
	let configuringEntry = $state<MCPCatalogEntry>();
	let configuringComponentId = $state<string>();
	let configuringComponent = $state<VMCPComponent>();
	let tools = $state<CompositeServerToolRow[]>([]);
	let toolPrefix = $state<string>();
	let modifyingExistingComponent = $state(false);
	let refreshToolsRequested = $state(false);

	const otherEffectiveNames = $derived(
		compositeEffectiveToolNames(
			(modifyingVMcp?.components ?? []).filter(
				(component) => componentId(component) !== configuringComponentId
			)
		)
	);
	const otherToolPrefixes = $derived(
		(modifyingVMcp?.components ?? [])
			.filter((component) => componentId(component) !== configuringComponentId)
			.map((component) => (component.toolPrefix ?? '').trim())
			.filter(Boolean)
	);
	const existingToolPrefix = $derived(
		(modifyingVMcp?.components ?? []).find(
			(component) => componentId(component) === configuringComponentId
		)?.toolPrefix
	);
	const excludedComponentIds = $derived([
		...(modifyingVMcp?.components ?? []).map(componentId),
		...(modifyingVMcp ? [modifyingVMcp.id] : [])
	]);

	function clearConfiguration() {
		modifyingVMcp = undefined;
		configuringEntry = undefined;
		configuringComponentId = undefined;
		configuringComponent = undefined;
		toolPrefix = undefined;
		tools = [];
		modifyingExistingComponent = false;
		refreshToolsRequested = false;
	}

	function close() {
		dialog = undefined;
		addedServer = undefined;
		clearConfiguration();
	}

	function configure(vmcp: VMCP, component: VMCPComponent, existing: boolean, refresh = false) {
		const id = componentId(component);
		if (!id || !component.catalogEntry?.manifest) {
			errors.append('Could not load this server to modify its tools.');
			return false;
		}
		refreshToolsRequested = refresh;
		modifyingExistingComponent = existing;
		modifyingVMcp = vmcp;
		configuringEntry = componentEntry(component);
		configuringComponentId = id;
		configuringComponent = component;
		toolPrefix = component.toolPrefix ?? '';
		tools = toolRowsFromComponent(component);
		return true;
	}

	function openSetup(vmcp: VMCP, component: VMCPComponent, existing = false, refresh = false) {
		if (configure(vmcp, component, existing, refresh)) dialog = 'setup';
	}

	function openEdit(vmcp: VMCP, component: VMCPComponent) {
		if (configure(vmcp, component, true)) dialog = 'edit';
	}

	function findComponent(vmcp: VMCP, id?: string) {
		return vmcp.components.find((candidate) => componentId(candidate) === id);
	}

	function openComponent(component: { id?: string }, vmcp: VMCP) {
		const raw = findComponent(vmcp, component.id);
		if (!raw) return;

		if (raw.toolOverrides?.length) {
			openEdit(vmcp, raw);
			return;
		}
		if (configure(vmcp, raw, true)) {
			dialog = 'actions';
		}
	}

	/** Skip the actions chooser: setup when there are no stored overrides, otherwise edit them. */
	function editComponent(component: { id?: string }, vmcp: VMCP) {
		const raw = findComponent(vmcp, component.id);
		if (!raw) return;
		if (raw.toolOverrides?.length) {
			openEdit(vmcp, raw);
			return;
		}
		openSetup(vmcp, raw, true);
	}

	/** Skip the actions chooser and prompt to remove this component from the vMCP. */
	function promptRemoveComponent(
		component: { id?: string; name?: string; description?: string; icon?: string },
		vmcp: VMCP
	) {
		if (!component.id) return;
		pendingRemoval = { component, vmcp };
		dialog = undefined;
	}

	function offerToolSelection(component: VMCPComponent, vmcp: VMCP, fromCreate = false) {
		addedServer = { component, vmcp };
		dialog = fromCreate ? 'added-create' : 'added-confirm';
	}

	function handleVMcpCreated(vmcp: VMCP) {
		const firstComponent = vmcp.components[0];
		if (firstComponent) offerToolSelection(firstComponent, vmcp, true);
	}

	function selectToolsForAdded() {
		const pending = addedServer;
		addedServer = undefined;
		dialog = undefined;
		if (!pending) return;

		const component = pending.vmcp.components.find(
			(candidate) => componentId(candidate) === componentId(pending.component)
		);
		if (!component) {
			errors.append('Could not find that server on the vMCP.');
			return;
		}
		openSetup(pending.vmcp, component);
	}

	function modifyToolsFromActions() {
		const vmcp = modifyingVMcp;
		const component = configuringComponent;
		if (vmcp && component) openSetup(vmcp, component, true);
	}

	function refreshTools() {
		const vmcp = modifyingVMcp;
		const component = configuringComponent;
		if (vmcp && component) openSetup(vmcp, component, true, true);
	}

	async function saveEditedTools() {
		const component = configuringComponent;
		if (!component) {
			close();
			return;
		}
		await saveTools({
			...component,
			toolPrefix,
			toolOverrides: toolOverridesFromRows(tools)
		});
	}

	async function saveTools(componentConfig: VMCPComponent) {
		const vmcpId = modifyingVMcp?.id;
		if (!vmcpId) {
			close();
			return;
		}

		try {
			const latest = await UserService.getVMCP(vmcpId);
			const id = componentId(componentConfig);
			const index = latest.components.findIndex((candidate) => componentId(candidate) === id);
			if (index < 0) {
				close();
				return;
			}
			const nextComponents = latest.components.map((component, componentIndex) =>
				componentIndex === index
					? { ...component, ...componentConfig, id: component.id ?? componentConfig.id }
					: component
			);
			const updated = await UserService.updateVMCP(vmcpId, vmcpManifest(latest, nextComponents));
			await options.onUpdated?.(updated);
			success.add(
				`Tools updated for ${componentConfig.catalogEntry.manifest.name} on ${updated.displayName}.`
			);
		} catch {
			errors.append('Failed to update tools for this vMCP.');
		} finally {
			close();
		}
	}

	function promptRemove() {
		if (!modifyingVMcp || !configuringComponent || !configuringEntry) return;
		pendingRemoval = {
			component: {
				id: configuringComponentId,
				name: configuringComponent.name || configuringEntry.manifest.name,
				description:
					configuringEntry.manifest.shortDescription || configuringEntry.manifest.description,
				icon: configuringEntry.manifest.icon
			},
			vmcp: modifyingVMcp
		};
		dialog = undefined;
	}

	function cancelRemove() {
		pendingRemoval = undefined;
		close();
	}

	async function removeComponent() {
		if (!pendingRemoval) return;
		const { component, vmcp } = pendingRemoval;
		if (vmcp.components.length <= 1) {
			errors.append('A vMCP must contain at least one component.');
			pendingRemoval = undefined;
			return;
		}

		removing = true;
		try {
			const latest = await UserService.getVMCP(vmcp.id);
			const nextComponents = latest.components.filter(
				(candidate) => componentId(candidate) !== component.id
			);
			if (nextComponents.length === latest.components.length) {
				close();
				return;
			}
			const updated = await UserService.updateVMCP(latest.id, vmcpManifest(latest, nextComponents));
			await options.onUpdated?.(updated);
			success.add(`${component.name} removed from ${updated.displayName}.`);
		} catch {
			errors.append('Failed to remove MCP server from vMCP.');
		} finally {
			removing = false;
			pendingRemoval = undefined;
			close();
		}
	}

	return {
		get dialog() {
			return dialog;
		},
		get addedServer() {
			return addedServer;
		},
		get pendingRemoval() {
			return pendingRemoval;
		},
		get removing() {
			return removing;
		},
		get modifyingVMcp() {
			return modifyingVMcp;
		},
		get configuringEntry() {
			return configuringEntry;
		},
		get configuringComponentId() {
			return configuringComponentId;
		},
		get configuringComponent() {
			return configuringComponent;
		},
		get tools() {
			return tools;
		},
		get toolPrefix() {
			return toolPrefix;
		},
		set toolPrefix(value: string | undefined) {
			toolPrefix = value;
		},
		get modifyingExistingComponent() {
			return modifyingExistingComponent;
		},
		get refresh() {
			return refreshToolsRequested;
		},
		get otherEffectiveNames() {
			return otherEffectiveNames;
		},
		get otherToolPrefixes() {
			return otherToolPrefixes;
		},
		get existingToolPrefix() {
			return existingToolPrefix;
		},
		get excludedComponentIds() {
			return excludedComponentIds;
		},
		close,
		openSetup,
		openEdit,
		openComponent,
		editComponent,
		offerToolSelection,
		handleVMcpCreated,
		selectToolsForAdded,
		modifyToolsFromActions,
		refreshTools,
		saveEditedTools,
		saveTools,
		promptRemove,
		promptRemoveComponent,
		cancelRemove,
		removeComponent
	};
}

export type VMcpToolFlow = ReturnType<typeof createVMcpToolFlow>;
