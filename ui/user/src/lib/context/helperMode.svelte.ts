import { getContext, hasContext, setContext } from 'svelte';

export const HELPER_TEXTS = {
	mcpServers: 'Incorporate MCP Servers to add tools/extend actions the agent can perform.',
	threads: 'These are current or prior conversations that have been made with the agent.',
	tasks:
		'Automated or one-off workflows that an agent can perform for the user. Incorporate variables to make them dynamic or make them simple enough to meet your needs.',

	general: 'Modify basic displayable information of the agent such as name, description, and icon.',
	prompt:
		'Describe how your agent should behave, what it should aim to do, and any special instructions it should follow.',
	introductions:
		'Begin every conversation with an introduction and default options that a user can choose from.',
	knowledge:
		'Add a collection of information (from documents to websites) that the agent can use to answer questions or perform tasks.',
	projectFiles: 'Add files that are available to use/view by a user with every conversation.',
	interfaces: 'Hook up an agent to third party services to automate tasks and workflows.',
	sharing:
		'Collaborate, share a simplified version to interact with, or make a template of the agent.',
	members: 'Modify who has access to collaborate on your agent.',
	chatbot: 'Share a simplified version of the agent for other users to interact with.',
	agentTemplate: 'Create a template to allow users to create their own version of the agent.',
	configuration: 'Make use of advanced features such as knowledge and more.',
	memories: 'Information stored by the agent to remember things mentioned in conversations.',
	modelProviders:
		'Configure popular model providers (OpenAI, Anthropic, etc.) and set a default model for the agent.',
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
