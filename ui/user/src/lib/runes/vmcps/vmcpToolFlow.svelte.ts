import { DEFAULT_MCP_CATALOG_ID } from '$lib/constants';
import {
	AdminService,
	type CatalogComponentServer,
	type CompositeServerToolRow,
	type MCPCatalogEntry,
	type MCPCatalogServer
} from '$lib/services';
import { compositeEffectiveToolNames, toolOverridesFromRows } from '$lib/services/user/mcp';
import { errors, mcpServersAndEntries } from '$lib/stores';
import { success } from '$lib/stores/success';

export type VMcpToolDialog = 'added-confirm' | 'added-create' | 'setup' | 'edit' | 'actions';

interface PendingAddedServer {
	component: MCPCatalogEntry;
	vmcp: MCPCatalogEntry;
}

interface PendingRemoval {
	component: {
		id?: string;
		name?: string;
		description?: string;
		icon?: string;
	};
	vmcp: MCPCatalogEntry;
}

function componentId(component: { catalogEntryID?: string; mcpServerID?: string }) {
	return component.catalogEntryID || component.mcpServerID || '';
}

function catalogEntryForComponent(component: CatalogComponentServer) {
	if (component.catalogEntryID) {
		return mcpServersAndEntries.current.entries.find(
			(entry) => entry.id === component.catalogEntryID
		);
	}

	if (!component.mcpServerID) return undefined;
	const server = mcpServersAndEntries.current.servers.find(
		(candidate) => candidate.id === component.mcpServerID
	);
	if (!server?.catalogEntryID) return undefined;
	return mcpServersAndEntries.current.entries.find((entry) => entry.id === server.catalogEntryID);
}

function resolveConfiguringEntry(
	component: CatalogComponentServer
): MCPCatalogEntry | MCPCatalogServer | undefined {
	if (component.mcpServerID) {
		return mcpServersAndEntries.current.servers.find(
			(server) => server.id === component.mcpServerID
		);
	}

	if (component.catalogEntryID) {
		const entry = mcpServersAndEntries.current.entries.find(
			(candidate) => candidate.id === component.catalogEntryID
		);
		if (entry) return component.manifest ? { ...entry, manifest: component.manifest } : entry;
	}

	const id = componentId(component);
	if (!id || !component.manifest) return undefined;
	return {
		id,
		created: new Date().toISOString(),
		manifest: component.manifest,
		sourceURL: undefined,
		userCount: undefined,
		type: 'catalog-entry',
		isCatalogEntry: !component.mcpServerID,
		needsUpdate: false
	};
}

function toolRowsFromComponent(component: CatalogComponentServer): CompositeServerToolRow[] {
	const id = componentId(component);
	const previewByName = new Map(
		(component.manifest?.toolPreview ?? []).map((tool) => [tool.name, tool])
	);
	return (component.toolOverrides ?? []).map((override) => {
		const previewTool = previewByName.get(override.name);
		const baseDescription = override.description ?? previewTool?.description;
		return {
			id: `${id}-${override.name}`,
			name: override.name,
			overrideName: (override.overrideName || '').trim() || override.name,
			description: baseDescription,
			overrideDescription: (override.overrideDescription || '').trim() || baseDescription,
			enabled: override.enabled !== false
		};
	});
}

/**
 * Owns the state machine for choosing, editing, refreshing, and removing tools on a vMCP.
 * Dialog rendering is kept in VMcpToolDialogs; this module owns transitions and persistence.
 */
export function createVMcpToolFlow() {
	let dialog = $state<VMcpToolDialog>();
	let addedServer = $state<PendingAddedServer>();
	let pendingRemoval = $state<PendingRemoval>();
	let removing = $state(false);
	let modifyingVMcp = $state<MCPCatalogEntry>();
	let configuringEntry = $state<MCPCatalogEntry | MCPCatalogServer>();
	let configuringComponentId = $state<string>();
	let configuringComponent = $state<CatalogComponentServer>();
	let tools = $state<CompositeServerToolRow[]>([]);
	let toolPrefix = $state<string>();
	let modifyingExistingComponent = $state(false);

	const otherEffectiveNames = $derived(
		compositeEffectiveToolNames(
			(modifyingVMcp?.manifest.compositeConfig?.componentServers ?? []).filter(
				(component) => componentId(component) !== configuringComponentId
			)
		)
	);
	const otherToolPrefixes = $derived(
		(modifyingVMcp?.manifest.compositeConfig?.componentServers ?? [])
			.filter((component) => componentId(component) !== configuringComponentId)
			.map((component) => (component.toolPrefix ?? '').trim())
			.filter(Boolean)
	);
	const existingToolPrefix = $derived(
		(modifyingVMcp?.manifest.compositeConfig?.componentServers ?? []).find(
			(component) => componentId(component) === configuringComponentId
		)?.toolPrefix
	);
	const excludedComponentIds = $derived([
		...(modifyingVMcp?.manifest.compositeConfig?.componentServers ?? []).map(componentId),
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
	}

	function close() {
		dialog = undefined;
		addedServer = undefined;
		clearConfiguration();
	}

	function configure(vmcp: MCPCatalogEntry, component: CatalogComponentServer, existing: boolean) {
		const id = componentId(component);
		const entry = resolveConfiguringEntry(component);
		if (!id || !entry) {
			errors.append('Could not load this server to modify its tools.');
			return false;
		}
		modifyingExistingComponent = existing;
		modifyingVMcp = vmcp;
		configuringEntry = entry;
		configuringComponentId = id;
		configuringComponent = component;
		toolPrefix = component.toolPrefix ?? '';
		tools = toolRowsFromComponent(component);
		return true;
	}

	function openSetup(vmcp: MCPCatalogEntry, component: CatalogComponentServer, existing = false) {
		if (configure(vmcp, component, existing)) dialog = 'setup';
	}

	function openEdit(vmcp: MCPCatalogEntry, component: CatalogComponentServer) {
		if (configure(vmcp, component, true)) dialog = 'edit';
	}

	function findComponent(vmcp: MCPCatalogEntry, id?: string) {
		return (vmcp.manifest.compositeConfig?.componentServers ?? []).find(
			(candidate) => componentId(candidate) === id
		);
	}

	function openComponent(component: { id?: string }, vmcp: MCPCatalogEntry) {
		const raw = findComponent(vmcp, component.id);
		if (!raw) return;

		if (raw.toolOverrides?.length) {
			openEdit(vmcp, raw);
			return;
		}
		if (configure(vmcp, raw, true)) {
			tools = [];
			dialog = 'actions';
		}
	}

	/** Skip the actions chooser: setup when there are no stored overrides, otherwise edit them. */
	function editComponent(component: { id?: string }, vmcp: MCPCatalogEntry) {
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
		vmcp: MCPCatalogEntry
	) {
		if (!component.id) return;
		pendingRemoval = { component, vmcp };
		dialog = undefined;
	}

	function offerToolSelection(
		component: MCPCatalogEntry,
		vmcp: MCPCatalogEntry,
		fromCreate = false
	) {
		addedServer = { component, vmcp };
		dialog = fromCreate ? 'added-create' : 'added-confirm';
	}

	function handleVMcpCreated(vmcp: MCPCatalogEntry) {
		const firstComponent = vmcp.manifest.compositeConfig?.componentServers?.[0];
		if (!firstComponent) return;
		const entry = catalogEntryForComponent(firstComponent);
		if (entry) {
			offerToolSelection(entry, vmcp, true);
		} else {
			openSetup(vmcp, firstComponent);
		}
	}

	function selectToolsForAdded() {
		const pending = addedServer;
		addedServer = undefined;
		dialog = undefined;
		if (!pending) return;

		const components = pending.vmcp.manifest.compositeConfig?.componentServers ?? [];
		const component =
			components.find((candidate) => candidate.catalogEntryID === pending.component.id) ??
			components.find((candidate) => {
				const server = mcpServersAndEntries.current.servers.find(
					(item) => item.id === candidate.mcpServerID
				);
				return server?.catalogEntryID === pending.component.id;
			});
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
		if (vmcp && component) openSetup(vmcp, component, true);
	}

	async function saveEditedTools() {
		const component = configuringComponent;
		if (!component || !configuringEntry) {
			close();
			return;
		}
		await saveTools({
			...component,
			toolPrefix,
			toolOverrides: toolOverridesFromRows(tools)
		});
	}

	async function saveTools(componentConfig: CatalogComponentServer) {
		const vmcpId = modifyingVMcp?.id;
		if (!vmcpId) {
			close();
			return;
		}

		try {
			const latest = await AdminService.getMCPCatalogEntry(DEFAULT_MCP_CATALOG_ID, vmcpId);
			if (!latest.manifest.compositeConfig) {
				close();
				return;
			}

			const id = componentId(componentConfig);
			const components = latest.manifest.compositeConfig.componentServers ?? [];
			const index = components.findIndex((candidate) => componentId(candidate) === id);
			const nextComponents =
				index >= 0
					? [
							...components.slice(0, index),
							{ ...components[index], ...componentConfig },
							...components.slice(index + 1)
						]
					: [...components, componentConfig];
			const updated = await AdminService.updateMCPCatalogEntry(DEFAULT_MCP_CATALOG_ID, latest.id, {
				...latest.manifest,
				compositeConfig: {
					...latest.manifest.compositeConfig,
					componentServers: nextComponents
				}
			});
			mcpServersAndEntries.current.entries = mcpServersAndEntries.current.entries.map(
				(candidate) => (candidate.id === updated.id ? updated : candidate)
			);
			success.add(
				`Tools updated for ${componentConfig.manifest?.name ?? 'this server'} on ${updated.manifest.name}.`
			);
		} catch {
			errors.append('Failed to update tools for this server.');
		} finally {
			close();
		}
	}

	function promptRemove() {
		if (!modifyingVMcp || !configuringEntry) return;
		pendingRemoval = {
			component: {
				id: configuringComponentId ?? configuringEntry.id,
				name: configuringEntry.manifest.name,
				description: configuringEntry.manifest.shortDescription,
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
		if (!vmcp.manifest.compositeConfig) return;

		removing = true;
		try {
			const latest = await AdminService.getMCPCatalogEntry(DEFAULT_MCP_CATALOG_ID, vmcp.id);
			if (!latest.manifest.compositeConfig) {
				close();
				return;
			}
			await AdminService.updateMCPCatalogEntry(DEFAULT_MCP_CATALOG_ID, vmcp.id, {
				...latest.manifest,
				compositeConfig: {
					...latest.manifest.compositeConfig,
					componentServers: latest.manifest.compositeConfig.componentServers?.filter(
						(candidate) =>
							candidate.catalogEntryID !== component.id && candidate.mcpServerID !== component.id
					)
				}
			});
			success.add(`${component.name} removed from ${latest.manifest.name}.`);
		} catch {
			errors.append('Failed to remove MCP server from vMCP.');
		} finally {
			removing = false;
			pendingRemoval = undefined;
			close();
			mcpServersAndEntries.refreshEntries();
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
		get existingToolPrefix() {
			return existingToolPrefix;
		},
		get otherEffectiveNames() {
			return otherEffectiveNames;
		},
		get otherToolPrefixes() {
			return otherToolPrefixes;
		},
		get excludedComponentIds() {
			return excludedComponentIds;
		},
		close,
		openComponent,
		editComponent,
		promptRemoveComponent,
		offerToolSelection,
		handleVMcpCreated,
		selectToolsForAdded,
		modifyToolsFromActions,
		refreshTools,
		saveEditedTools,
		saveTools,
		promptRemove,
		cancelRemove,
		removeComponent
	};
}

export type VMcpToolFlow = ReturnType<typeof createVMcpToolFlow>;
