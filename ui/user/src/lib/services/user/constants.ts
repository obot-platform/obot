export const MCP_CONNECTION_INVALID_LICENSE_MESSAGE =
	"We're sorry, this system is currently operating with limited functionality. Please contact your administrator.";

export enum AiClient {
	Claude = 'claude',
	Codex = 'codex',
	Cursor = 'cursor',
	VSCode = 'vscode',
	OpenClaw = 'openclaw',
	OpenCode = 'open-code',
	Hermes = 'hermes',
	Goose = 'goose',
	Zed = 'zed',
	Windsurf = 'windsurf'
}

export const COMMON_AI_CLIENTS = [
	{
		id: AiClient.Claude,
		icon: '/user/images/assistant/claude-mark.svg',
		alt: 'Claude'
	},
	{
		id: AiClient.Codex,
		icon: '/user/images/assistant/codex-mark.svg',
		alt: 'Codex'
	},
	{
		id: AiClient.Cursor,
		icon: '/user/images/assistant/cursor-mark.svg',
		iconDark: '/user/images/assistant/cursor-mark-dark.svg',
		alt: 'Cursor'
	},
	{
		id: AiClient.VSCode,
		icon: '/user/images/assistant/vscode-mark.svg',
		alt: 'VS Code'
	},
	{
		id: AiClient.OpenClaw,
		icon: '/user/images/assistant/openclaw-mark.svg',
		alt: 'OpenClaw'
	},
	{
		id: AiClient.OpenCode,
		icon: '/user/images/assistant/opencode-mark.svg',
		iconDark: '/user/images/assistant/opencode-mark-dark.svg',
		alt: 'Open Code'
	},
	{
		id: AiClient.Hermes,
		icon: '/user/images/assistant/hermes-mark.svg',
		iconDark: '/user/images/assistant/hermes-mark-dark.svg',
		alt: 'Hermes'
	},
	{
		id: AiClient.Goose,
		icon: '/user/images/assistant/goose-mark.svg',
		iconDark: '/user/images/assistant/goose-mark-dark.svg',
		alt: 'Goose'
	},
	{
		id: AiClient.Zed,
		icon: '/user/images/assistant/zed-mark.svg',
		iconDark: '/user/images/assistant/zed-mark-dark.svg',
		alt: 'Zed'
	},
	{
		id: AiClient.Windsurf,
		icon: '/user/images/assistant/windsurf-mark.svg',
		iconDark: '/user/images/assistant/windsurf-mark-dark.svg',
		alt: 'Windsurf'
	}
];

export const MAGIC_LINK_SUPPORTED_AI_CLIENTS = [AiClient.Cursor, AiClient.VSCode];
export const COMMAND_SUPPORTED_AI_CLIENTS = [AiClient.Claude, AiClient.Codex];

export const AUDIT_LOG_FILTER_OPTIONS_LIMIT = 1000;
