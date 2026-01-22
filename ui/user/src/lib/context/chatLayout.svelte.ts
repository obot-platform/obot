import { TASK_NEW_ID } from '$lib/constants';
import type { ProjectMCP, Task, TaskRun, Thread } from '$lib/services';
import type { EditorItem } from '$lib/services/editor/index.svelte';
import { responsive } from '$lib/stores';
import { getContext, hasContext, setContext } from 'svelte';

export const CHAT_LAYOUT_CONTEXT = 'chat-layout';

export interface Layout {
	sidebarOpen?: boolean;
	editTaskID?: string;
	displayTaskRun?: TaskRun;
	tasks?: Task[];
	threads?: Thread[];
	taskRuns?: Thread[];
	items: EditorItem[];
	projectEditorOpen?: boolean;
	fileEditorOpen?: boolean;
	sidebarConfig?:
		| 'introduction'
		| 'system-prompt'
		| 'slack'
		| 'chatbot'
		| 'discord'
		| 'sms'
		| 'email'
		| 'webhook'
		| 'template'
		| 'knowledge'
		| 'custom-tool'
		| 'invitations'
		| 'model-providers'
		| 'mcp-server-tools'
		| 'mcp-server'
		| 'mcpserver-interface'
		| 'project-configuration';

	editProjectMcp?: ProjectMCP;
	mcpServer?: ProjectMCP;
	chatbotMcpEdit?: boolean;
	sidebarMemoryUpdateAvailable?: boolean;
	newChatMode?: boolean;
	deleting?: boolean;
	input?: string;
	liveProjectEditing?: {
		filename: string;
		content: string;
	};
}

export function isSomethingSelected(layout: Layout) {
	return layout.editTaskID || layout.displayTaskRun || layout.editProjectMcp || layout.mcpServer;
}

export function closeAll(layout: Layout) {
	if (layout.sidebarConfig === 'project-configuration') {
		// remove edit from url
		// eslint-disable-next-line svelte/prefer-svelte-reactivity
		const url = new URL(window.location.href);
		url.searchParams.delete('edit');
		window.history.replaceState({}, '', url.toString());
	}
	layout.editTaskID = undefined;
	layout.displayTaskRun = undefined;
	layout.sidebarConfig = undefined;
	layout.editProjectMcp = undefined;
	layout.mcpServer = undefined;
	layout.chatbotMcpEdit = undefined;
}

let newTaskCounter = 0;

export function createNewTask(layout: Layout) {
	newTaskCounter++;
	const task = {
		id: `${TASK_NEW_ID}-${newTaskCounter}`,
		name: 'New Task',
		steps: [{ id: 'si1' + Math.random().toString(36).substring(6) }]
	};
	if (!layout.tasks) {
		layout.tasks = [];
	}
	layout.tasks.push(task);
	openTask(layout, task.id);
}

export function openTask(layout: Layout, taskID?: string) {
	closeAll(layout);
	layout.editTaskID = taskID;
}

export function openTaskRun(layout: Layout, taskRun?: TaskRun) {
	closeAll(layout);
	layout.displayTaskRun = taskRun;
}

export function openSidebarConfig(layout: Layout, config: Layout['sidebarConfig']) {
	closeAll(layout);
	layout.fileEditorOpen = false;
	layout.sidebarConfig = config;
	if (responsive.isMobile) {
		layout.sidebarOpen = false;
	}
}

export function openMCPServerTools(layout: Layout, mcpServer: ProjectMCP) {
	closeAll(layout);
	layout.fileEditorOpen = false;
	layout.sidebarConfig = 'mcp-server-tools';
	layout.mcpServer = mcpServer;
	if (responsive.isMobile) {
		layout.sidebarOpen = false;
	}
}

export function openMCPServer(layout: Layout, mcpServer: ProjectMCP) {
	closeAll(layout);
	layout.fileEditorOpen = false;
	layout.sidebarConfig = 'mcp-server';
	layout.mcpServer = mcpServer;
	if (responsive.isMobile) {
		layout.sidebarOpen = false;
	}
}
export function closeSidebarConfig(layout: Layout) {
	layout.sidebarConfig = undefined;
	layout.editProjectMcp = undefined;
	layout.mcpServer = undefined;
	layout.chatbotMcpEdit = undefined;
}

export function initLayout(layout: Layout) {
	const data = $state<Layout>(layout);
	setContext(CHAT_LAYOUT_CONTEXT, data);
}

export function getLayout(): Layout {
	if (!hasContext(CHAT_LAYOUT_CONTEXT)) {
		throw new Error('chat layout context not initialized');
	}
	return getContext<Layout>(CHAT_LAYOUT_CONTEXT);
}
