import {
	UserService,
	type CompositeServerToolRow,
	type MCPCatalogEntry,
	type ToolOverride,
	type VMCP,
	type VMCPComponent,
	type VMCPConfigurationPolicy
} from '$lib/services';
import { compositeEffectiveToolNames, toolOverridesFromRows } from '$lib/services/user/mcp';
import {
	catalogConfigurationFields,
	vmcpComponentId,
	vmcpManifest
} from '$lib/services/vmcps/utils';
import { errors } from '$lib/stores';
import { success } from '$lib/stores/success';

export type VMcpToolDialog = 'added-create' | 'setup' | 'edit' | 'actions' | 'configure';

interface PendingAddedServer {
	component: MCPCatalogEntry;
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

/**
 * Handover from the page that creates a vMCP to the page that shows it. Creating navigates to the
 * new vMCP, which unmounts the flow that would have opened the tool dialogs, so the id is parked
 * here for the next flow to claim once it is mounted.
 */
let vmcpAwaitingToolSetup: string | undefined;
let vmcpAwaitingProfilesHint: string | undefined;
let vmcpCreateHandoffPending = $state(false);

export const VMCP_PROFILES_HINT_STORAGE_KEY = '@obot/seen-vmcp-profiles-hint';

export function hasSeenVMcpProfilesHint(storageKey = VMCP_PROFILES_HINT_STORAGE_KEY): boolean {
	try {
		return Boolean(localStorage.getItem(storageKey));
	} catch {
		return false;
	}
}

export function markVMcpProfilesHintSeen(storageKey = VMCP_PROFILES_HINT_STORAGE_KEY) {
	try {
		// eslint-disable-next-line svelte/prefer-svelte-reactivity
		localStorage.setItem(storageKey, new Date().toISOString());
	} catch {
		// Ignore storage failures in restricted contexts.
	}
}

export function queueToolSetupForCreatedVMcp(id: string) {
	vmcpAwaitingToolSetup = id;
	vmcpCreateHandoffPending = true;
}

/** Claims the queued setup, if it is for this vMCP. Only ever succeeds once per creation. */
export function claimToolSetupForVMcp(id: string) {
	if (!id || vmcpAwaitingToolSetup !== id) return false;
	vmcpAwaitingToolSetup = undefined;
	return true;
}

export function peekQueuedToolSetupVMcp() {
	return vmcpAwaitingToolSetup;
}

export function isVMcpCreateHandoffPending() {
	return vmcpCreateHandoffPending;
}

export function finishVMcpCreateHandoff() {
	vmcpCreateHandoffPending = false;
}

export function queueProfilesHintForCreatedVMcp(id: string) {
	if (hasSeenVMcpProfilesHint()) return;
	vmcpAwaitingProfilesHint = id;
}

export function claimProfilesHintForVMcp(id: string) {
	if (hasSeenVMcpProfilesHint()) return false;
	if (!id || vmcpAwaitingProfilesHint !== id) return false;
	vmcpAwaitingProfilesHint = undefined;
	return true;
}

function catalogEntryForComponent(component: VMCPComponent): MCPCatalogEntry | undefined {
	const manifest = component.catalogEntry?.manifest;
	if (!manifest) return undefined;
	return {
		id: component.mcpServerCatalogEntryID,
		created: new Date(0).toISOString(),
		manifest,
		isCatalogEntry: true,
		type: 'catalog-entry',
		unsupportedTools: component.catalogEntry?.unsupportedTools
	};
}

function toolRowsFromComponent(component: VMCPComponent): CompositeServerToolRow[] {
	const id = vmcpComponentId(component);
	const preview = component.catalogEntry?.manifest?.toolPreview ?? [];
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

/**
 * Owns the state machine for choosing, editing, refreshing, and removing tools on a vMCP.
 * Dialog rendering is kept in VMcpToolDialogs; this module owns transitions and persistence.
 */
export function createVMcpToolFlow() {
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
	let collecting = $state(false);
	let collectTools: ((config: VMCPComponent) => void) | undefined;
	let onVMcpChanged: ((vmcp: VMCP) => void) | undefined;
	let postCreateConfiguration = $state(false);

	const otherEffectiveNames = $derived(
		compositeEffectiveToolNames(
			(modifyingVMcp?.components ?? []).filter(
				(component) => vmcpComponentId(component) !== configuringComponentId
			)
		)
	);
	const otherToolPrefixes = $derived(
		(modifyingVMcp?.components ?? [])
			.filter((component) => vmcpComponentId(component) !== configuringComponentId)
			.map((component) => (component.toolPrefix ?? '').trim())
			.filter(Boolean)
	);
	const existingToolPrefix = $derived(
		(modifyingVMcp?.components ?? []).find(
			(component) => vmcpComponentId(component) === configuringComponentId
		)?.toolPrefix
	);
	const excludedComponentIds = $derived([
		...(modifyingVMcp?.components ?? []).map(vmcpComponentId),
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
		collecting = false;
		collectTools = undefined;
		postCreateConfiguration = false;
	}

	function close() {
		dialog = undefined;
		addedServer = undefined;
		clearConfiguration();
	}

	function configure(vmcp: VMCP, component: VMCPComponent, existing: boolean, refresh = false) {
		const id = vmcpComponentId(component);
		const entry = catalogEntryForComponent(component);
		if (!id || !entry) {
			errors.append('Could not load this server to modify its tools.');
			return false;
		}
		refreshToolsRequested = refresh;
		modifyingExistingComponent = existing;
		modifyingVMcp = vmcp;
		configuringEntry = entry;
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
		return (vmcp.components ?? []).find((candidate) => vmcpComponentId(candidate) === id);
	}

	/**
	 * Fetches this component's tools, opens the tool editor, and returns the chosen
	 * overrides without writing them onto the vMCP.
	 */
	function collectComponentTools(
		component: VMCPComponent,
		vmcp: VMCP,
		onCollected: (config: VMCPComponent) => void
	) {
		if (!configure(vmcp, component, true)) return;
		collecting = true;
		collectTools = onCollected;
		dialog = 'setup';
	}

	function openComponent(component: { id?: string }, vmcp: VMCP) {
		const raw = findComponent(vmcp, component.id);
		if (!raw) return;
		if (configure(vmcp, raw, true)) dialog = 'actions';
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
		if (!component.id || (vmcp.components ?? []).length <= 1) return;
		pendingRemoval = { component, vmcp };
		dialog = undefined;
	}

	function offerToolSelection(component: MCPCatalogEntry, vmcp: VMCP) {
		addedServer = { component, vmcp };
		dialog = 'added-create';
	}

	function offerToolsAfterCreate(vmcp: VMCP, component: VMCPComponent, entry?: MCPCatalogEntry) {
		if (component.configuration?.some((field) => field.policy === 'userAllowed')) return;
		if (entry) {
			offerToolSelection(entry, vmcp);
			return;
		}
		openSetup(vmcp, component);
	}

	function handleVMcpCreated(vmcp: VMCP) {
		const firstComponent = vmcp.components?.[0];
		if (!firstComponent) return;
		const entry = catalogEntryForComponent(firstComponent);
		if (
			entry &&
			catalogConfigurationFields(entry).length > 0 &&
			configure(vmcp, firstComponent, false)
		) {
			postCreateConfiguration = true;
			dialog = 'configure';
			return;
		}
		offerToolsAfterCreate(vmcp, firstComponent, entry);
	}

	function selectToolsForAdded() {
		const pending = addedServer;
		addedServer = undefined;
		dialog = undefined;
		if (!pending) return;

		const component = (pending.vmcp.components ?? []).find(
			(candidate) =>
				candidate.mcpServerCatalogEntryID === pending.component.id ||
				vmcpComponentId(candidate) === pending.component.id
		);
		if (!component) {
			errors.append('Could not find that server on the vMCP.');
			return;
		}
		openSetup(pending.vmcp, component);
	}

	function editConfiguration() {
		if (!configuringEntry) return;
		dialog = 'configure';
	}

	function returnToActions() {
		if (postCreateConfiguration) {
			close();
			return;
		}
		if (configuringComponent && modifyingVMcp) {
			dialog = 'actions';
			return;
		}
		close();
	}

	async function saveConfiguration(
		configuration: VMCPConfigurationPolicy[],
		forceSingleUser: boolean
	) {
		const component = configuringComponent;
		if (!component) {
			close();
			return;
		}
		const vmcpId = modifyingVMcp?.id;
		if (!vmcpId) {
			close();
			return;
		}

		try {
			const latest = await UserService.getVMCP(vmcpId);
			const id = vmcpComponentId(component);
			const components = latest.components ?? [];
			const index = components.findIndex((candidate) => vmcpComponentId(candidate) === id);
			if (index < 0) {
				close();
				return;
			}
			const nextComponents = components.map((candidate, componentIndex) =>
				componentIndex === index
					? {
							...candidate,
							...component,
							configuration,
							forceSingleUser,
							id: candidate.id ?? component.id
						}
					: candidate
			);
			const updated = await UserService.updateVMCP(latest.id, {
				...vmcpManifest(latest),
				components: nextComponents
			});
			modifyingVMcp = updated;
			success.add(
				`Configuration updated for ${component.catalogEntry?.manifest?.name ?? component.name ?? 'this server'} on ${updated.displayName}.`
			);
			onVMcpChanged?.(updated);
			if (postCreateConfiguration) {
				const entry = configuringEntry;
				const saved = findComponent(updated, vmcpComponentId(component));
				postCreateConfiguration = false;
				if (saved && !saved.configuration?.some((field) => field.policy === 'userAllowed')) {
					offerToolsAfterCreate(updated, saved, entry);
					return;
				}
				close();
				return;
			}
			close();
		} catch {
			errors.append('Failed to update configuration for this vMCP.');
			throw new Error('Failed to update configuration for this vMCP.');
		}
	}

	function modifyToolsFromActions() {
		const vmcp = modifyingVMcp;
		const component = configuringComponent;
		if (!vmcp || !component) return;
		if (component.configuration?.some((field) => field.policy === 'userAllowed')) return;
		if (component.toolOverrides?.length) {
			openEdit(vmcp, component);
			return;
		}
		openSetup(vmcp, component, true);
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
		if (collectTools) {
			const done = collectTools;
			collectTools = undefined;
			done(componentConfig);
			close();
			return;
		}

		const vmcpId = modifyingVMcp?.id;
		if (!vmcpId) {
			close();
			return;
		}

		try {
			const latest = await UserService.getVMCP(vmcpId);
			const id = vmcpComponentId(componentConfig);
			const components = latest.components ?? [];
			const index = components.findIndex((candidate) => vmcpComponentId(candidate) === id);
			if (index < 0) {
				close();
				return;
			}
			const nextComponents = components.map((component, componentIndex) =>
				componentIndex === index
					? { ...component, ...componentConfig, id: component.id ?? componentConfig.id }
					: component
			);
			const updated = await UserService.updateVMCP(latest.id, {
				...vmcpManifest(latest),
				components: nextComponents
			});
			modifyingVMcp = updated;
			success.add(
				`Tools updated for ${componentConfig.catalogEntry?.manifest?.name ?? componentConfig.name ?? 'this server'} on ${updated.displayName}.`
			);
			onVMcpChanged?.(updated);
		} catch {
			errors.append('Failed to update tools for this vMCP.');
		} finally {
			close();
		}
	}

	function promptRemove() {
		if (!modifyingVMcp || !configuringComponent || !configuringEntry) return;
		if ((modifyingVMcp.components ?? []).length <= 1) return;
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
		const lastComponentWarning =
			'Cannot remove the last remaining component. Connecting to a vMCP requires at least one component.';
		if ((vmcp.components ?? []).length <= 1) {
			errors.append(lastComponentWarning);
			pendingRemoval = undefined;
			return;
		}

		removing = true;
		try {
			const latest = await UserService.getVMCP(vmcp.id);
			if ((latest.components ?? []).length <= 1) {
				errors.append(lastComponentWarning);
				return;
			}
			const updated = await UserService.updateVMCP(latest.id, {
				...vmcpManifest(latest),
				components: (latest.components ?? []).filter(
					(candidate) =>
						vmcpComponentId(candidate) !== component.id &&
						candidate.mcpServerCatalogEntryID !== component.id
				)
			});
			success.add(`${component.name} removed from ${updated.displayName}.`);
			onVMcpChanged?.(updated);
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
		get collecting() {
			return collecting;
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
		get canConfigureComponent() {
			return Boolean(configuringEntry);
		},
		get postCreateConfiguration() {
			return postCreateConfiguration;
		},
		setOnVMcpChanged(handler: ((vmcp: VMCP) => void) | undefined) {
			onVMcpChanged = handler;
		},
		close,
		collectComponentTools,
		openSetup,
		openEdit,
		openComponent,
		editComponent,
		promptRemoveComponent,
		offerToolSelection,
		handleVMcpCreated,
		selectToolsForAdded,
		editConfiguration,
		returnToActions,
		saveConfiguration,
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
