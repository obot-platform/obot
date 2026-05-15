<script lang="ts">
	const options = [
		{
			id: 'cursor',
			label: 'Cursor',
			icon: '/user/images/assistant/cursor-mark.svg',
			url: 'https://cursor.com/docs/mcp'
		},
		{
			id: 'claudeDesktop',
			label: 'Claude Desktop',
			icon: '/user/images/assistant/claude-mark.svg',
			url: 'https://support.claude.com/en/articles/11175166-get-started-with-custom-connectors-using-remote-mcp'
		},
		{
			id: 'claudeCode',
			label: 'Claude Code',
			icon: '/user/images/assistant/claude-mark.svg',
			url: 'https://code.claude.com/docs/en/mcp'
		},
		{
			id: 'vscode',
			label: 'VSCode',
			icon: '/user/images/assistant/vscode-mark.svg',
			url: 'https://code.visualstudio.com/docs/copilot/customization/mcp-servers'
		}
	];
</script>

<div class="w-full overflow-hidden border-b border-base-300 dark:border-base-400 mb-4">
	<div class="flex gap-4 flex-col mb-4">
		<p class="text-sm font-light w-md max-w-full text-center self-center">
			For more documentation on how to set up your MCP server, click the appropriate resource below:
		</p>
		{#each options as option (option.id)}
			<a
				href={option.url}
				target="_blank"
				rel="noopener noreferrer external"
				class="flex uppercase font-medium font-mono text-xs items-center gap-2 self-center bg-base-300 dark:bg-base-100 hover:bg-base-400 dark:hover:bg-base-200 rounded-xs px-4 py-2 w-64 justify-center"
			>
				<span class="p-0.5 rounded-sm dark:bg-base-300">
					<img src={option.icon} alt={`${option.label} branding icon`} class="size-4" />
				</span>
				{option.label}
			</a>
		{/each}
	</div>
</div>
