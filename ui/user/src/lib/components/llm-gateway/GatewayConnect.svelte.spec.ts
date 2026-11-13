import type { Model } from '$lib/services';
import GatewayConnect from './GatewayConnect.svelte';
import { describe, expect, it, vi } from 'vitest';
import { render } from 'vitest-browser-svelte';
import { page } from 'vitest/browser';

const obotURL = 'https://obot.example.com';

const models = [
	{
		id: 'm1',
		name: 'gpt',
		targetModel: 'openai/gpt-4o',
		dialect: 'OpenAIResponses'
	},
	{
		id: 'm2',
		name: 'claude',
		targetModel: 'anthropic/claude-opus-4-5',
		dialect: 'AnthropicMessages'
	}
] as unknown as Model[];

function openClient(name: string) {
	return page.getByRole('button', { name }).click();
}

function openTab(name: string) {
	return page.getByRole('tab', { name }).click();
}

async function pickModel(label: string, picker = page.getByRole('combobox', { name: 'Model' })) {
	await picker.click();
	await page.getByRole('button', { name: label, exact: true }).click();
}

describe('GatewayConnect', () => {
	it('shows a card for every client', async () => {
		render(GatewayConnect, { obotURL });

		for (const [name, description] of [
			['Claude Code', 'Use any gateway model from Claude Code'],
			['Codex CLI', "OpenAI's coding agent, through the Responses API"],
			['OpenAI SDK', 'Python and TypeScript clients for Chat Completions and Responses'],
			['Anthropic SDK', 'Python and TypeScript clients for the Messages API'],
			['Google ADK', "Google's Python framework for building agents"],
			['Vercel AI SDK', 'TypeScript toolkit for building AI apps and agents'],
			['curl', 'Call the gateway directly over HTTP'],
			['OpenCode', 'Open-source coding agent for the terminal'],
			['Goose', 'Open-source agent for the desktop and terminal'],
			['Cline', 'Open-source coding agent for VS Code and JetBrains'],
			['Open WebUI', 'Self-hosted chat interface for any gateway model']
		]) {
			const card = page.getByRole('button', { name });
			await expect.element(card).toBeVisible();
			await expect.element(card).toHaveAccessibleDescription(description);
		}
		// No setup is shown until a client is picked.
		await expect
			.element(page.getByText('export ANTHROPIC_BASE_URL', { exact: false }))
			.not.toBeInTheDocument();
	});

	it('filters the client list by name', async () => {
		render(GatewayConnect, { obotURL });

		await page.getByPlaceholder('Search clients...').fill('curl');
		await expect
			.element(page.getByRole('button', { name: 'Claude Code', exact: true }))
			.not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'curl', exact: true })).toBeVisible();
	});

	it('filters the client list by description', async () => {
		render(GatewayConnect, { obotURL });

		await page.getByPlaceholder('Search clients...').fill('JetBrains');
		await expect
			.element(page.getByRole('button', { name: 'curl', exact: true }))
			.not.toBeInTheDocument();
		await expect.element(page.getByRole('button', { name: 'Cline', exact: true })).toBeVisible();
	});

	it('says when no client matches the search', async () => {
		render(GatewayConnect, { obotURL });

		await page.getByPlaceholder('Search clients...').fill('no-such-client');
		await expect
			.element(page.getByRole('button', { name: 'Cline', exact: true }))
			.not.toBeInTheDocument();
		await expect.element(page.getByText('No clients match "no-such-client".')).toBeVisible();
	});

	it('opens Claude Code setup pointed at this Obot server', async () => {
		render(GatewayConnect, { obotURL });

		await openClient('Claude Code');

		await expect.element(page.getByRole('dialog')).toBeVisible();
		await expect.element(page.getByRole('heading', { name: 'Claude Code' })).toBeVisible();
		await expect
			.element(
				page.getByText('export ANTHROPIC_BASE_URL="https://obot.example.com/api/llm-proxy"', {
					exact: false
				})
			)
			.toBeInTheDocument();
		await expect
			.element(
				page.getByText('export ANTHROPIC_AUTH_TOKEN="[REPLACE_WITH_API_KEY]"', {
					exact: false
				})
			)
			.toBeInTheDocument();
		// Claude Code picks its own Claude models; no model is set.
		await expect
			.element(page.getByText('ANTHROPIC_MODEL', { exact: false }))
			.not.toBeInTheDocument();
		await expect
			.element(
				page.getByText('export CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=1', { exact: false })
			)
			.toBeInTheDocument();
		// Without Claude subscriptions allowed, there is only the API key setup.
		await expect
			.element(page.getByText('ANTHROPIC_CUSTOM_HEADERS', { exact: false }))
			.not.toBeInTheDocument();
	});

	it('leaves the model to Claude Code when the gateway has Anthropic models', async () => {
		render(GatewayConnect, { obotURL, models });

		await openClient('Claude Code');
		await expect.element(page.getByText('Run in a terminal', { exact: true })).toBeVisible();
		await expect
			.element(page.getByText('export ANTHROPIC_BASE_URL', { exact: false }))
			.toBeInTheDocument();
		await expect
			.element(page.getByText('ANTHROPIC_MODEL', { exact: false }))
			.not.toBeInTheDocument();
	});

	it("sets Claude Code's starting model from the picker", async () => {
		render(GatewayConnect, { obotURL, models });

		await openClient('Claude Code');
		const picker = page.getByRole('combobox', { name: 'Model' });
		await expect.element(picker).toHaveTextContent("Claude Code's default");

		await pickModel('OpenAI · openai/gpt-4o');
		await expect
			.element(page.getByText('export ANTHROPIC_MODEL="openai/gpt-4o"', { exact: false }))
			.toBeInTheDocument();
		await expect
			.element(page.getByText('no Anthropic provider', { exact: false }))
			.not.toBeInTheDocument();

		await pickModel("Claude Code's default");
		await expect
			.element(page.getByText('ANTHROPIC_MODEL', { exact: false }))
			.not.toBeInTheDocument();
	});

	it('only offers Anthropic models for the Claude subscription setup', async () => {
		render(GatewayConnect, { obotURL, models, subscriptionPassthrough: true });

		await openClient('Claude Code');
		await expect.element(page.getByRole('tab', { name: 'Use subscription' })).toBeVisible();
		const subscription = page.getByRole('combobox', { name: 'Model' });
		await expect
			.element(page.getByText('Only Anthropic models use your Claude subscription.'))
			.toBeVisible();

		await subscription.click();
		await expect
			.element(page.getByRole('button', { name: 'Anthropic · anthropic/claude-opus-4-5' }))
			.toBeVisible();
		await expect
			.element(page.getByRole('button', { name: 'OpenAI · openai/gpt-4o' }))
			.not.toBeInTheDocument();
		await page.getByRole('button', { name: 'Anthropic · anthropic/claude-opus-4-5' }).click();
		await expect
			.element(
				page.getByText('export ANTHROPIC_MODEL="anthropic/claude-opus-4-5"', { exact: false })
			)
			.toBeInTheDocument();

		// The API key setup has its own choice, from every model.
		await openTab('Use API key');
		await pickModel('OpenAI · openai/gpt-4o');
		await expect
			.element(page.getByText('export ANTHROPIC_MODEL="openai/gpt-4o"', { exact: false }))
			.toBeInTheDocument();
	});

	it('names a gateway model for Claude Code without an Anthropic provider', async () => {
		const bedrockOnly = [
			{
				id: 'm3',
				name: 'embed',
				targetModel: 'bedrock/amazon.titan-embed-text-v2:0',
				usage: 'text-embedding'
			},
			{
				id: 'm4',
				name: 'nova',
				targetModel: 'bedrock/amazon.nova-pro-v1:0',
				usage: 'llm'
			},
			{
				id: 'm5',
				name: 'claude',
				targetModel: 'bedrock/anthropic.claude-sonnet-4-5-20250929-v1:0',
				usage: 'llm'
			}
		] as unknown as Model[];
		render(GatewayConnect, { obotURL, models: bedrockOnly });

		await openClient('Claude Code');
		await expect
			.element(
				page.getByText(
					'export ANTHROPIC_MODEL="bedrock/anthropic.claude-sonnet-4-5-20250929-v1:0"',
					{
						exact: false
					}
				)
			)
			.toBeInTheDocument();
	});

	it('shows the Claude subscription setup when subscriptions are allowed', async () => {
		render(GatewayConnect, { obotURL, subscriptionPassthrough: true });

		await openClient('Claude Code');
		await expect.element(page.getByRole('tab', { name: 'Use subscription' })).toBeVisible();
		await expect
			.element(page.getByText('With a Claude subscription (Pro, Max, Team or Enterprise)'))
			.toBeInTheDocument();
		await expect
			.element(
				page.getByText('export ANTHROPIC_CUSTOM_HEADERS="X-Obot-Api-Key: [REPLACE_WITH_API_KEY]"', {
					exact: false
				})
			)
			.toBeInTheDocument();
		await expect
			.element(page.getByText('unset ANTHROPIC', { exact: false }))
			.not.toBeInTheDocument();
		await openTab('Use API key');
		await expect.element(page.getByText('With an Obot API key')).toBeInTheDocument();
	});

	it('uses the first Anthropic model in the Anthropic SDK example', async () => {
		render(GatewayConnect, { obotURL, models });

		await openClient('Anthropic SDK');
		await expect.element(page.getByText('Run in a terminal', { exact: true })).toBeVisible();
		await expect
			.element(page.getByText('model="anthropic/claude-opus-4-5"', { exact: false }))
			.toBeInTheDocument();
	});

	it('prints the text of an Anthropic SDK reply that starts with thinking', async () => {
		render(GatewayConnect, { obotURL, models });

		await openClient('Anthropic SDK');
		await expect
			.element(
				page.getByText('print(next(b.text for b in m.content if b.type == "text"))', {
					exact: false
				})
			)
			.toBeInTheDocument();
		await expect
			.element(page.getByText('m.content[0].text', { exact: false }))
			.not.toBeInTheDocument();
	});

	it('points Open WebUI in Docker at the Obot server', async () => {
		render(GatewayConnect, { obotURL, models });

		await openClient('Open WebUI');
		await expect
			.element(
				page.getByText('OPENAI_API_BASE_URL="https://obot.example.com/api/llm-proxy/v1"', {
					exact: false
				})
			)
			.toBeInTheDocument();
		await expect
			.element(page.getByText('host.docker.internal', { exact: false }))
			.not.toBeInTheDocument();
	});

	it('points Open WebUI in Docker at an Obot on localhost through the Docker host', async () => {
		render(GatewayConnect, { obotURL: 'http://localhost:8080', models });

		await openClient('Open WebUI');
		await expect
			.element(
				page.getByText('OPENAI_API_BASE_URL="http://host.docker.internal:8080/api/llm-proxy/v1"', {
					exact: false
				})
			)
			.toBeInTheDocument();
		await expect
			.element(page.getByText('--add-host=host.docker.internal:host-gateway', { exact: false }))
			.toBeInTheDocument();
	});

	it('gives the manual Open WebUI steps the address Docker reaches an Obot on localhost at', async () => {
		render(GatewayConnect, { obotURL: 'http://localhost:8080', models });

		await openClient('Open WebUI');
		const steps = page.getByRole('dialog').getByRole('listitem');
		const urlStep = steps.filter({ hasText: 'URL:' }).first();
		await expect
			.element(
				urlStep.getByText('http://host.docker.internal:8080/api/llm-proxy/v1', { exact: true })
			)
			.toBeVisible();
		// Open WebUI run without Docker reaches Obot at localhost.
		await expect.element(urlStep.getByText(/without Docker, use this URL instead/)).toBeVisible();
		await expect
			.element(urlStep.getByText('http://localhost:8080/api/llm-proxy/v1', { exact: true }))
			.toBeVisible();
	});

	it('gives the manual Open WebUI steps the Obot URL when it is not on localhost', async () => {
		render(GatewayConnect, { obotURL, models });

		await openClient('Open WebUI');
		const urlStep = page
			.getByRole('dialog')
			.getByRole('listitem')
			.filter({ hasText: 'URL:' })
			.first();
		await expect
			.element(urlStep.getByText('https://obot.example.com/api/llm-proxy/v1', { exact: true }))
			.toBeVisible();
		await expect.element(urlStep.getByText(/without Docker/)).not.toBeInTheDocument();
	});

	it('rewrites the OpenAI SDK example with the picked model', async () => {
		render(GatewayConnect, { obotURL, models });

		await openClient('OpenAI SDK');
		await expect.element(page.getByText('Run in a terminal', { exact: true })).toBeVisible();
		await expect
			.element(page.getByText('model="openai/gpt-4o"', { exact: false }))
			.toBeInTheDocument();

		await pickModel('Anthropic · anthropic/claude-opus-4-5');
		await expect
			.element(page.getByText('model="anthropic/claude-opus-4-5"', { exact: false }))
			.toBeInTheDocument();
		await expect
			.element(page.getByText('model="openai/gpt-4o"', { exact: false }))
			.not.toBeInTheDocument();
	});

	it('points a Google ADK agent at the gateway with the picked model', async () => {
		render(GatewayConnect, { obotURL, models });

		await openClient('Google ADK');
		await expect.element(page.getByText('Install in a terminal', { exact: true })).toBeVisible();
		await expect.element(page.getByText('obot_agent/agent.py')).toBeInTheDocument();
		await expect.element(page.getByText('Run in a terminal', { exact: true })).toBeVisible();
		await expect
			.element(
				page.getByText('api_base="https://obot.example.com/api/llm-proxy/v1"', { exact: false })
			)
			.toBeInTheDocument();
		// LiteLLM's openai/ prefix comes before the gateway model's own provider prefix.
		await expect
			.element(page.getByText('model="openai/openai/gpt-4o"', { exact: false }))
			.toBeInTheDocument();

		await pickModel('Anthropic · anthropic/claude-opus-4-5');
		await expect
			.element(page.getByText('model="openai/anthropic/claude-opus-4-5"', { exact: false }))
			.toBeInTheDocument();
		await expect
			.element(page.getByText('model="openai/openai/gpt-4o"', { exact: false }))
			.not.toBeInTheDocument();
	});

	it('points the Vercel AI SDK at the gateway with the picked model', async () => {
		render(GatewayConnect, { obotURL, models });

		await openClient('Vercel AI SDK');
		await expect.element(page.getByText('Install in a terminal', { exact: true })).toBeVisible();
		await expect.element(page.getByText('Run in a terminal', { exact: true })).toBeVisible();
		await expect
			.element(
				page.getByText("baseURL: 'https://obot.example.com/api/llm-proxy/v1'", { exact: false })
			)
			.toBeInTheDocument();
		await expect
			.element(page.getByText("model: obot('openai/gpt-4o')", { exact: false }))
			.toBeInTheDocument();

		await pickModel('Anthropic · anthropic/claude-opus-4-5');
		await expect
			.element(page.getByText("model: obot('anthropic/claude-opus-4-5')", { exact: false }))
			.toBeInTheDocument();
		await expect
			.element(page.getByText("model: obot('openai/gpt-4o')", { exact: false }))
			.not.toBeInTheDocument();
	});

	it("turns off Codex's web search for models OpenAI doesn't serve", async () => {
		render(GatewayConnect, { obotURL, models });

		await openClient('Codex CLI');
		await expect.element(page.getByText('web_search', { exact: false })).not.toBeInTheDocument();

		await pickModel('Anthropic · anthropic/claude-opus-4-5');
		await expect
			.element(page.getByText('model = "anthropic/claude-opus-4-5"', { exact: false }))
			.toBeInTheDocument();
		await expect
			.element(page.getByText('web_search = "disabled"', { exact: false }))
			.toBeInTheDocument();
	});

	it('configures Codex with an OpenAI-format model', async () => {
		render(GatewayConnect, { obotURL, models });

		await openClient('Codex CLI');
		await expect.element(page.getByText('~/.codex/config.toml')).toBeInTheDocument();
		await expect.element(page.getByText('Run in a terminal', { exact: true })).toBeVisible();
		await expect
			.element(page.getByText('model = "openai/gpt-4o"', { exact: false }))
			.toBeInTheDocument();
		await expect
			.element(
				page.getByText('base_url = "https://obot.example.com/api/llm-proxy/v1"', { exact: false })
			)
			.toBeInTheDocument();
	});

	it('configures OpenCode with the gateway and the picked model', async () => {
		render(GatewayConnect, { obotURL, models });

		await openClient('OpenCode');
		await expect.element(page.getByText('~/.config/opencode/opencode.json')).toBeInTheDocument();
		await expect.element(page.getByText('Run in a terminal', { exact: true })).toBeVisible();
		await expect
			.element(
				page.getByText('"baseURL": "https://obot.example.com/api/llm-proxy/v1"', { exact: false })
			)
			.toBeInTheDocument();
		await expect
			.element(page.getByText('"model": "obot/openai/gpt-4o"', { exact: false }))
			.toBeInTheDocument();

		await pickModel('Anthropic · anthropic/claude-opus-4-5');
		await expect
			.element(page.getByText('"model": "obot/anthropic/claude-opus-4-5"', { exact: false }))
			.toBeInTheDocument();
		await expect
			.element(page.getByText('"anthropic/claude-opus-4-5": {', { exact: false }))
			.toBeInTheDocument();
	});

	it("splits the gateway URL into Goose's host and path", async () => {
		render(GatewayConnect, { obotURL, models });

		await openClient('Goose');
		await expect.element(page.getByText('Run in a terminal', { exact: true })).toBeVisible();
		await expect
			.element(page.getByText('export OPENAI_HOST="https://obot.example.com"', { exact: false }))
			.toBeInTheDocument();
		await expect
			.element(
				page.getByText('export OPENAI_BASE_PATH="api/llm-proxy/v1/chat/completions"', {
					exact: false
				})
			)
			.toBeInTheDocument();
		await expect
			.element(page.getByText('export GOOSE_MODEL="openai/gpt-4o"', { exact: false }))
			.toBeInTheDocument();
	});

	it('lists the settings to enter in Cline', async () => {
		render(GatewayConnect, { obotURL, models });

		await openClient('Cline');
		const dialog = page.getByRole('dialog');
		await expect
			.element(dialog.getByText('https://obot.example.com/api/llm-proxy/v1', { exact: true }))
			.toBeVisible();
		await expect.element(dialog.getByText('openai/gpt-4o', { exact: true })).toBeVisible();

		await pickModel('Anthropic · anthropic/claude-opus-4-5');
		await expect
			.element(dialog.getByText('anthropic/claude-opus-4-5', { exact: true }))
			.toBeVisible();
		await expect
			.element(
				dialog.getByText('obot login --url https://obot.example.com --scope llm --print-token', {
					exact: true
				})
			)
			.toBeVisible();
	});

	it('shows a generated access token in its own field and keeps it out of the snippets', async () => {
		const apiKey = 'ok1-generated';
		render(GatewayConnect, { obotURL, models, generatedApiKey: apiKey });

		await openClient('Claude Code');
		const dialog = page.getByRole('dialog');
		await expect.element(dialog.getByRole('textbox', { name: 'Your API Key' })).toHaveValue(apiKey);
		await expect
			.element(
				dialog.getByText('export ANTHROPIC_AUTH_TOKEN="[REPLACE_WITH_API_KEY]"', {
					exact: false
				})
			)
			.toBeInTheDocument();
		await expect
			.element(dialog.getByText('[REPLACE_WITH_API_KEY]', { exact: true }))
			.toHaveClass(/bg-primary\/25/);
		await expect
			.element(dialog.getByText(`ANTHROPIC_AUTH_TOKEN="${apiKey}"`, { exact: false }))
			.not.toBeInTheDocument();
		await expect
			.element(dialog.getByText('obot login --url', { exact: false }))
			.not.toBeInTheDocument();
	});

	it('defaults the snippet to a key placeholder and toggles it to the CLI command', async () => {
		render(GatewayConnect, { obotURL });

		await openClient('Claude Code');
		const dialog = page.getByRole('dialog');
		const command = '$(obot login --url https://obot.example.com --scope llm --print-token)';

		await expect.element(dialog.getByText('[REPLACE_WITH_API_KEY]', { exact: true })).toBeVisible();
		await expect
			.element(
				dialog.getByText(
					'Supply your existing API key for ANTHROPIC_AUTH_TOKEN or generate a new one.',
					{ exact: true }
				)
			)
			.toBeVisible();

		await dialog.getByRole('button', { name: 'CLI', exact: true }).click();
		await expect.element(dialog.getByText(command, { exact: true })).toBeVisible();
		await expect
			.element(dialog.getByText('[REPLACE_WITH_API_KEY]', { exact: true }))
			.not.toBeInTheDocument();
		await expect
			.element(
				dialog.getByText(
					'Supply your existing API key for ANTHROPIC_AUTH_TOKEN or generate a new one.',
					{ exact: true }
				)
			)
			.not.toBeInTheDocument();

		await dialog.getByRole('button', { name: 'Key', exact: true }).click();
		await expect.element(dialog.getByText('[REPLACE_WITH_API_KEY]', { exact: true })).toBeVisible();
	});

	function keyNotice(variable: string) {
		return page
			.getByRole('dialog')
			.getByText(`Supply your existing API key for ${variable} or generate a new one.`, {
				exact: true
			});
	}

	it('names the variable of the Claude Code tab shown in the existing-key notice', async () => {
		render(GatewayConnect, { obotURL, models, subscriptionPassthrough: true });

		await openClient('Claude Code');
		await expect.element(keyNotice('X-Obot-Api-Key')).toBeVisible();
		await openTab('Use API key');
		await expect.element(keyNotice('ANTHROPIC_AUTH_TOKEN')).toBeVisible();
	});

	it('names the variable a client snippet reads the key from in the existing-key notice', async () => {
		render(GatewayConnect, { obotURL, models });

		await openClient('Codex CLI');
		await expect.element(keyNotice('OBOT_API_KEY')).toBeVisible();
	});

	it('leaves the variable out of the existing-key notice for clients set up in their own settings', async () => {
		render(GatewayConnect, { obotURL, models });

		await openClient('Cline');
		const dialog = page.getByRole('dialog');
		await expect
			.element(
				dialog.getByText('Supply your existing API key or generate a new one.', { exact: true })
			)
			.toBeVisible();
		await expect
			.element(
				dialog.getByText('API Key: paste your generated API key or run the CLI command below.')
			)
			.toBeVisible();
	});

	it('asks the page to create a token from the existing-key notice', async () => {
		const onGenerateApiKey = vi.fn().mockResolvedValue(true);
		render(GatewayConnect, { obotURL, hasValidIdentityApiKeys: true, onGenerateApiKey });

		await openClient('Claude Code');
		await page.getByRole('dialog').getByRole('button', { name: 'Generate New Key' }).click();

		expect(onGenerateApiKey).toHaveBeenCalledOnce();
	});

	it('keeps a generated token in its own field when an identity key already exists', async () => {
		const apiKey = 'ok1-generated';
		render(GatewayConnect, { obotURL, generatedApiKey: apiKey, hasValidIdentityApiKeys: true });

		await openClient('Codex CLI');
		const dialog = page.getByRole('dialog');
		await expect.element(dialog.getByRole('textbox', { name: 'Your API Key' })).toHaveValue(apiKey);
		await expect
			.element(dialog.getByText('export OBOT_API_KEY="[REPLACE_WITH_API_KEY]"', { exact: false }))
			.toBeInTheDocument();
		await expect
			.element(dialog.getByText(`OBOT_API_KEY="${apiKey}"`, { exact: false }))
			.not.toBeInTheDocument();
		await expect
			.element(dialog.getByRole('button', { name: 'Key', exact: true }))
			.not.toBeInTheDocument();
		await expect
			.element(dialog.getByText('Supply your existing API key', { exact: false }))
			.not.toBeInTheDocument();
	});

	it('shows the generated token where Cline asks for the login command', async () => {
		const apiKey = 'ok1-generated';
		render(GatewayConnect, { obotURL, models, generatedApiKey: apiKey });

		await openClient('Cline');
		const dialog = page.getByRole('dialog');
		await expect.element(dialog.getByText('API Key: copy the key below.')).toBeVisible();
		await expect.element(dialog.getByRole('textbox', { name: 'Your API Key' })).toHaveValue(apiKey);
		await expect
			.element(dialog.getByText('obot login --url', { exact: false }))
			.not.toBeInTheDocument();
	});

	it('shows curl requests for all three API formats', async () => {
		render(GatewayConnect, { obotURL });

		await openClient('curl');

		await expect.element(page.getByRole('heading', { name: 'curl' })).toBeVisible();
		await expect.element(page.getByRole('tab', { name: 'Via Chat Completions' })).toBeVisible();
		await expect.element(page.getByText('OpenAI Chat Completions (any model)')).toBeVisible();
		await openTab('Via Responses');
		await expect.element(page.getByText('OpenAI Responses (any model)')).toBeVisible();
		await expect
			.element(
				page.getByText('curl https://obot.example.com/api/llm-proxy/v1/responses', { exact: false })
			)
			.toBeInTheDocument();
		await expect.element(page.getByText('"input": "hello"', { exact: false })).toBeInTheDocument();
		await openTab('Via Messages');
		await expect.element(page.getByText('Anthropic Messages (any model)')).toBeVisible();
		await expect
			.element(page.getByText('export ANTHROPIC_BASE_URL', { exact: false }))
			.not.toBeInTheDocument();
	});
});
