import type { ProjectMCP, ProjectTemplate, Task, TaskRun, Thread } from '$lib/services';
import type { EditorItem } from '$lib/services/editor/index.svelte';
import { responsive } from '$lib/stores';
import { getContext, hasContext, setContext } from 'svelte';

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
		| 'custom-mcp'
		| 'model-providers'
		| 'mcp-server-tools'
		| 'mcpserver-interface';
	customToolId?: string;
	editProjectMcp?: ProjectMCP;
	template?: ProjectTemplate;
	mcpServer?: ProjectMCP;
	chatbotMcpEdit?: boolean;
}

export function isSomethingSelected(layout: Layout) {
	return layout.editTaskID || layout.displayTaskRun;
}

export function closeAll(layout: Layout) {
	layout.editTaskID = undefined;
	layout.displayTaskRun = undefined;
	layout.sidebarConfig = undefined;
	layout.customToolId = undefined;
	layout.editProjectMcp = undefined;
	layout.template = undefined;
	layout.mcpServer = undefined;
	layout.chatbotMcpEdit = undefined;
}

export function openTask(layout: Layout, taskID?: string) {
	closeAll(layout);
	layout.editTaskID = taskID;
}

export function openTaskRun(layout: Layout, taskRun?: TaskRun) {
	closeAll(layout);
	layout.displayTaskRun = taskRun;
}

export function openTemplate(layout: Layout, template: ProjectTemplate) {
	closeAll(layout);
	layout.sidebarConfig = 'template';
	layout.template = template;
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
}

export function openCustomTool(layout: Layout, customToolId: string) {
	closeAll(layout);
	layout.fileEditorOpen = false;
	layout.sidebarConfig = 'custom-tool';
	layout.customToolId = customToolId;
	if (responsive.isMobile) {
		layout.sidebarOpen = false;
	}
}

export function openEditProjectMcp(layout: Layout, projectMcp?: ProjectMCP, chatbot?: boolean) {
	closeAll(layout);
	layout.fileEditorOpen = false;
	layout.sidebarConfig = 'custom-mcp';
	layout.editProjectMcp = projectMcp;
	layout.chatbotMcpEdit = chatbot;
	if (responsive.isMobile) {
		layout.sidebarOpen = false;
	}
}

export function closeSidebarConfig(layout: Layout) {
	layout.sidebarConfig = undefined;
	layout.customToolId = undefined;
	layout.editProjectMcp = undefined;
	layout.template = undefined;
	layout.mcpServer = undefined;
	layout.chatbotMcpEdit = undefined;
}

export function initLayout(layout: Layout) {
	const data = $state<Layout>(layout);
	setContext('layout', data);
}

export function getLayout(): Layout {
	if (!hasContext('layout')) {
		throw new Error('layout context not initialized');
	}
	return getContext<Layout>('layout');
}
