import { getContext, hasContext, setContext } from 'svelte';

export const HELPER_TEXTS = {
	mcpServers: 'Incorporate MCP Servers to add tools/extend actions the project can perform.',
	threads: 'These are current or prior conversations that have been made with the project.',
	tasks:
		'Automated or one-off workflows that a project can perform for the user. Incorporate variables to make them dynamic or make them simple enough to meet your needs.',

	general:
		'Modify basic displayable information of the project such as name, description, and icon.',
	prompt:
		'Describe how your project should behave, what it should aim to do, and any special instructions it should follow.',
	introductions:
		'Begin every conversation with an introduction and default options that a user can choose from.',
	knowledge:
		'Add a collection of information (from documents to websites) that the project can use to answer questions or perform tasks.',
	projectFiles: 'Add files that are available to use/view by a user with every conversation.',
	interfaces: 'Hook up a project to third party services to automate tasks and workflows.',
	sharing:
		'Collaborate, share a simplified version to interact with, or make a template of the project.',
	members: 'Modify who has access to collaborate on your project.',
	chatbot: 'Share a simplified version of the project for other users to interact with.',
	agentTemplate: 'Create a template to allow users to create their own version of the project.',
	configuration: 'Make use of advanced features such as knowledge and more.',
	memories: 'Information stored by the project to remember things mentioned in conversations.',
	modelProviders:
		'Configure popular model providers (OpenAI, Anthropic, etc.) and set a default model for the project.',
	builtInCapabilities:
		'Obot provided capabilities such as using provided documents to expand information it has access to (Knowledge), or to remember things a user has requested in conversation (Memory).'
};

const HELPER_MODE_CONTEXT_NAME = 'helper-mode';
export interface HelperMode {
	isEnabled: boolean;
}

export function getHelperMode(): HelperMode {
	if (!hasContext(HELPER_MODE_CONTEXT_NAME)) {
		throw new Error('helper mode context not initialized');
	}
	return getContext<HelperMode>(HELPER_MODE_CONTEXT_NAME);
}

export function initHelperMode() {
	const helperMode = $state<HelperMode>({
		isEnabled: false
	});
	setContext(HELPER_MODE_CONTEXT_NAME, helperMode);
}
