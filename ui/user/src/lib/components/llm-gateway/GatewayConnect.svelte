<script lang="ts">
	import CopyButton from '$lib/components/CopyButton.svelte';
	import ResponsiveDialog from '$lib/components/ResponsiveDialog.svelte';
	import Select from '$lib/components/Select.svelte';
	import {
		gatewayModelNames,
		groupGatewayModels
	} from '$lib/components/llm-gateway/GatewayModelsDialog.svelte';
	import LLMGatewayCodeBlock from '$lib/components/llm-gateway/LLMGatewayCodeBlock.svelte';
	import { m } from '$lib/i18n';
	import type { Model } from '$lib/services';
	import type { SnippetBlock } from '$lib/services/llm-gateway/types';
	import { Terminal } from '@lucide/svelte';
	import { twMerge } from 'tailwind-merge';

	interface Props {
		obotURL: string;
		models?: Model[];
		// Whether an administrator allows Claude subscriptions through the gateway.
		subscriptionPassthrough?: boolean;
		class?: string;
	}

	let {
		obotURL,
		models = [],
		subscriptionPassthrough = false,
		class: klass = ''
	}: Props = $props();

	type Client =
		| 'claude-code'
		| 'codex'
		| 'openai-sdk'
		| 'anthropic-sdk'
		| 'google-adk'
		| 'vercel-ai-sdk'
		| 'curl'
		| 'opencode'
		| 'goose'
		| 'cline'
		| 'open-webui';
	let selected = $state<Client>();
	let dialog = $state<ReturnType<typeof ResponsiveDialog>>();

	let baseURL = $derived(`${obotURL}/api/llm-proxy`);
	// Obot's URL as a container on this machine reaches it, when that differs: inside a
	// container, localhost is the container itself, so an Obot on localhost is reached at
	// host.docker.internal.
	let containerObotURL = $derived.by(() => {
		try {
			const { protocol, hostname, port, pathname } = new URL(obotURL);
			if (!['localhost', '127.0.0.1', '[::1]', '0.0.0.0'].includes(hostname)) {
				return undefined;
			}
			const path = pathname.replace(/\/$/, '');
			return `${protocol}//host.docker.internal${port ? `:${port}` : ''}${path}`;
		} catch {
			return undefined;
		}
	});
	let loginCommand = $derived(`obot login --url ${obotURL} --scope llm --print-token`);
	let token = $derived(`$(${loginCommand})`);
	// The models a chat example can use: embedding and image models can't answer a message.
	let chatModels = $derived.by(() => {
		const chat = models.filter(
			(m) => m.usage !== 'text-embedding' && m.usage !== 'image-generation'
		);
		return chat.length > 0 ? chat : models;
	});
	let chatNames = $derived(gatewayModelNames(chatModels));
	let hasAnthropic = $derived(models.some((m) => m.targetModel?.startsWith('anthropic/')));
	let anthropicModel = $derived(
		chatNames.find((name) => name.startsWith('anthropic/')) ??
			chatModels.find((m) => m.dialect === 'AnthropicMessages')?.targetModel ??
			chatNames[0] ??
			'anthropic/claude-sonnet-4-5'
	);
	// Claude Code asks for Claude models by Anthropic's own names, which only resolve to the
	// gateway's Anthropic provider. Without one (a Bedrock-, Azure- or OpenAI-only gateway),
	// the setup names a gateway model for Claude Code to use instead, preferring a Claude model.
	let claudeCodeModel = $derived.by(() => {
		if (models.length === 0 || hasAnthropic) {
			return undefined;
		}
		const llm = models.filter((m) => !m.usage || m.usage === 'llm');
		return (
			llm.find((m) => m.targetModel?.includes('claude'))?.targetModel ??
			llm[0]?.targetModel ??
			models[0]?.targetModel
		);
	});
	let openaiModel = $derived(
		chatNames.find((name) => name.startsWith('openai/')) ??
			chatModels.find((m) => m.dialect !== 'AnthropicMessages')?.targetModel ??
			chatNames[0] ??
			'openai/gpt-4.1'
	);

	// The model pickers in a client's setup. Claude Code has one per setup, since only Claude
	// models use a Claude subscription; every other client has one for the whole setup.
	type PickerKey = 'model' | 'claude-code' | 'claude-code-subscription';
	// Leaves the model to Claude Code, which starts with its own default.
	const CLAUDE_CODE_DEFAULT = 'claude-code-default';
	let picked = $state<Partial<Record<PickerKey, string>>>({});

	let modelOptions = $derived(
		groupGatewayModels(chatModels).flatMap((group) =>
			group.names.map((name) => ({ id: name, label: `${group.label} · ${name}` }))
		)
	);
	let claudeCodeDefaultOption = $derived({
		id: CLAUDE_CODE_DEFAULT,
		label: m.models_connect_claude_code_default()
	});
	let pickerOptions = $derived<Record<PickerKey, { id: string; label: string }[]>>({
		model: modelOptions,
		// Without an Anthropic provider, Claude Code needs a gateway model named.
		'claude-code': hasAnthropic ? [claudeCodeDefaultOption, ...modelOptions] : modelOptions,
		'claude-code-subscription': [
			claudeCodeDefaultOption,
			...modelOptions.filter((o) => o.id.startsWith('anthropic/'))
		]
	});
	let pickerDefaults = $derived<Record<PickerKey, string>>({
		model: selected === 'anthropic-sdk' ? anthropicModel : openaiModel,
		'claude-code': claudeCodeModel ?? CLAUDE_CODE_DEFAULT,
		'claude-code-subscription': CLAUDE_CODE_DEFAULT
	});

	function pickedModel(key: PickerKey) {
		return picked[key] ?? pickerDefaults[key];
	}

	function showPicker(key: PickerKey) {
		return pickerOptions[key].length > 1;
	}

	// monochrome: a dark single-color logo, inverted in dark mode so it shows on a dark card.
	const clients: {
		id: Client;
		label: string;
		description: string;
		icon?: string;
		monochrome?: boolean;
	}[] = [
		{
			id: 'claude-code',
			label: 'Claude Code',
			description: m.models_connect_claude_code_description(),
			icon: '/admin/assets/anthropic_icon.svg',
			monochrome: true
		},
		{
			id: 'codex',
			label: 'Codex CLI',
			description: m.models_connect_codex_description(),
			icon: '/admin/assets/openai_icon.svg',
			monochrome: true
		},
		{
			id: 'opencode',
			label: 'OpenCode',
			description: m.models_connect_opencode_description(),
			icon: '/admin/assets/opencode_icon.svg',
			monochrome: true
		},
		{
			id: 'goose',
			label: 'Goose',
			description: m.models_connect_goose_description(),
			icon: '/admin/assets/goose_icon.svg',
			monochrome: true
		},
		{
			id: 'cline',
			label: 'Cline',
			description: m.models_connect_cline_description(),
			icon: '/admin/assets/cline_icon.svg',
			monochrome: true
		},
		{
			id: 'open-webui',
			label: 'Open WebUI',
			description: m.models_connect_open_webui_description()
		},
		{
			id: 'openai-sdk',
			label: 'OpenAI SDK',
			description: m.models_connect_openai_sdk_description(),
			icon: '/admin/assets/openai_icon.svg',
			monochrome: true
		},
		{
			id: 'anthropic-sdk',
			label: 'Anthropic SDK',
			description: m.models_connect_anthropic_sdk_description(),
			icon: '/admin/assets/anthropic_icon.svg',
			monochrome: true
		},
		{
			id: 'vercel-ai-sdk',
			label: 'Vercel AI SDK',
			description: m.models_connect_vercel_ai_sdk_description(),
			icon: '/admin/assets/vercel_ai_sdk_icon.svg'
		},
		{
			id: 'google-adk',
			label: 'Google ADK',
			description: m.models_connect_google_adk_description(),
			icon: '/admin/assets/google_adk_icon.svg'
		},
		{
			id: 'curl',
			label: 'curl',
			description: m.models_connect_curl_description()
		}
	];

	let selectedClient = $derived(clients.find((c) => c.id === selected));

	function open(client: Client) {
		selected = client;
		picked = {};
		dialog?.open();
	}

	// Claude Code 2.1.129 and later can list the gateway's models in its /model picker.
	const claudeCodeDiscovery = [
		"# List this gateway's models in Claude Code's /model picker (Claude Code 2.1.129+)",
		'export CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=1'
	];

	function claudeCodeModelLines(model: string) {
		if (model === CLAUDE_CODE_DEFAULT) return [];
		return [
			...(hasAnthropic
				? []
				: ['# This gateway has no Anthropic provider, so name one of its models:']),
			`export ANTHROPIC_MODEL="${model}"`
		];
	}

	type ConnectBlock = SnippetBlock & { picker?: PickerKey };

	// Clients set up in their own settings screens get numbered steps, each with an optional
	// value to copy into a field.
	// A step may end with a note, with a value of its own, such as another URL to use instead.
	type Step = { text: string; value?: string; note?: string; noteValue?: string };

	let steps = $derived.by((): Step[] => {
		const model = pickedModel('model');
		switch (selected) {
			case 'cline':
				return [
					{ text: m.models_connect_cline_step_provider() },
					{ text: m.models_connect_step_base_url(), value: `${baseURL}/v1` },
					{ text: m.models_connect_step_api_key() },
					{ text: m.models_connect_cline_step_model(), value: model }
				];
			case 'open-webui':
				return [
					{ text: m.models_connect_open_webui_step_connection() },
					// Open WebUI mostly runs in Docker, where an Obot on localhost is reached at
					// host.docker.internal, as in the Docker command below.
					containerObotURL
						? {
								text: m.models_connect_open_webui_step_url(),
								value: `${containerObotURL}/api/llm-proxy/v1`,
								note: m.models_connect_open_webui_step_url_docker_note(),
								noteValue: `${baseURL}/v1`
							}
						: { text: m.models_connect_open_webui_step_url(), value: `${baseURL}/v1` },
					{ text: m.models_connect_step_api_key() },
					{ text: m.models_connect_open_webui_step_model(), value: model }
				];
			default:
				return [];
		}
	});

	let apiKeyBlock = $derived<ConnectBlock>({
		title: m.models_connect_print_api_key(),
		language: 'bash',
		code: loginCommand
	});

	let blocks = $derived.by((): ConnectBlock[] => {
		const model = pickedModel('model');
		switch (selected) {
			case 'claude-code': {
				const apiKey = {
					picker: 'claude-code' as const,
					language: 'bash' as const,
					code: [
						`export ANTHROPIC_BASE_URL="${baseURL}"`,
						`export ANTHROPIC_AUTH_TOKEN="${token}"`,
						...claudeCodeDiscovery,
						...claudeCodeModelLines(pickedModel('claude-code')),
						'claude'
					].join('\n')
				};
				if (!subscriptionPassthrough) {
					return [apiKey];
				}
				return [
					{
						title: m.models_connect_with_subscription(),
						picker: 'claude-code-subscription' as const,
						language: 'bash' as const,
						code: [
							`export ANTHROPIC_BASE_URL="${baseURL}"`,
							`export ANTHROPIC_CUSTOM_HEADERS="X-Obot-Api-Key: ${token}"`,
							...claudeCodeDiscovery,
							...claudeCodeModelLines(pickedModel('claude-code-subscription')),
							'claude'
						].join('\n')
					},
					{ ...apiKey, title: m.models_connect_with_api_key() }
				];
			}
			case 'codex':
				return [
					{
						title: '~/.codex/config.toml',
						language: 'toml' as const,
						code: [
							`model = "${model}"`,
							`model_provider = "obot"`,
							// Codex's web search is a tool OpenAI runs, so only OpenAI's models have it.
							...(model.startsWith('openai/')
								? []
								: ['# Web search only works with OpenAI models', 'web_search = "disabled"']),
							'',
							'[model_providers.obot]',
							'name = "Obot LLM Gateway"',
							`base_url = "${baseURL}/v1"`,
							'env_key = "OBOT_API_KEY"',
							'wire_api = "responses"'
						].join('\n')
					},
					{
						language: 'bash' as const,
						code: `export OBOT_API_KEY="${token}"\ncodex`
					}
				];
			case 'openai-sdk':
				return [
					{
						language: 'bash' as const,
						code: [
							`export OPENAI_BASE_URL="${baseURL}/v1"`,
							`export OPENAI_API_KEY="${token}"`,
							'',
							'python - <<PY',
							'from openai import OpenAI',
							'client = OpenAI()',
							`r = client.chat.completions.create(model="${model}", messages=[{"role": "user", "content": "hello"}])`,
							'print(r.choices[0].message.content)',
							'PY'
						].join('\n')
					}
				];
			case 'anthropic-sdk':
				return [
					{
						language: 'bash' as const,
						code: [
							`export ANTHROPIC_BASE_URL="${baseURL}"`,
							`export ANTHROPIC_AUTH_TOKEN="${token}"`,
							'',
							'python - <<PY',
							'import anthropic',
							'client = anthropic.Anthropic()',
							`m = client.messages.create(model="${model}", max_tokens=512, messages=[{"role": "user", "content": "hello"}])`,
							'# Reasoning models answer with thinking blocks before the text',
							'print(next(b.text for b in m.content if b.type == "text"))',
							'PY'
						].join('\n')
					}
				];
			case 'google-adk':
				return [
					{
						language: 'bash' as const,
						code: [
							'pip install google-adk litellm',
							'mkdir -p obot_agent && echo "from . import agent" > obot_agent/__init__.py'
						].join('\n')
					},
					{
						title: 'obot_agent/agent.py',
						language: 'python' as const,
						code: [
							'import os',
							'',
							'from google.adk.agents import Agent',
							'from google.adk.models.lite_llm import LiteLlm',
							'',
							'root_agent = Agent(',
							'    name="assistant",',
							"    # LiteLLM's openai/ prefix, then the gateway model name",
							'    model=LiteLlm(',
							`        model="openai/${model}",`,
							`        api_base="${baseURL}/v1",`,
							'        api_key=os.environ["OBOT_API_KEY"],',
							'    ),',
							'    instruction="You are a helpful assistant.",',
							')'
						].join('\n')
					},
					{
						language: 'bash' as const,
						code: `export OBOT_API_KEY="${token}"\nadk run obot_agent`
					}
				];
			case 'vercel-ai-sdk':
				return [
					{
						language: 'bash' as const,
						code: 'npm install ai @ai-sdk/openai-compatible'
					},
					{
						title: 'index.mts',
						language: 'typescript' as const,
						code: [
							"import { createOpenAICompatible } from '@ai-sdk/openai-compatible';",
							"import { generateText } from 'ai';",
							'',
							'const obot = createOpenAICompatible({',
							"  name: 'obot',",
							`  baseURL: '${baseURL}/v1',`,
							'  apiKey: process.env.OBOT_API_KEY',
							'});',
							'',
							'const { text } = await generateText({',
							`  model: obot('${model}'),`,
							"  prompt: 'hello'",
							'});',
							'console.log(text);'
						].join('\n')
					},
					{
						language: 'bash' as const,
						code: `export OBOT_API_KEY="${token}"\nnpx tsx index.mts`
					}
				];
			case 'curl':
				return [
					{
						title: m.models_connect_chat_completions_any_model(),
						language: 'bash' as const,
						code: [
							`curl ${baseURL}/v1/chat/completions \\`,
							`  -H "Authorization: Bearer ${token}" \\`,
							'  -H "content-type: application/json" \\',
							`  -d '{"model": "${model}", "messages": [{"role": "user", "content": "hello"}]}'`
						].join('\n')
					},
					{
						title: m.models_connect_messages_any_model(),
						language: 'bash' as const,
						code: [
							`curl ${baseURL}/v1/messages \\`,
							`  -H "Authorization: Bearer ${token}" \\`,
							'  -H "anthropic-version: 2023-06-01" \\',
							'  -H "content-type: application/json" \\',
							`  -d '{"model": "${model}", "max_tokens": 512, "messages": [{"role": "user", "content": "hello"}]}'`
						].join('\n')
					}
				];
			case 'opencode':
				return [
					{
						title: '~/.config/opencode/opencode.json',
						language: 'json' as const,
						code: JSON.stringify(
							{
								$schema: 'https://opencode.ai/config.json',
								model: `obot/${model}`,
								provider: {
									obot: {
										npm: '@ai-sdk/openai-compatible',
										name: 'Obot LLM Gateway',
										options: {
											baseURL: `${baseURL}/v1`,
											apiKey: '{env:OBOT_API_KEY}'
										},
										models: { [model]: { name: model } }
									}
								}
							},
							null,
							2
						)
					},
					{
						language: 'bash' as const,
						code: `export OBOT_API_KEY="${token}"\nopencode`
					}
				];
			case 'goose':
				return [
					{
						language: 'bash' as const,
						code: [
							'export GOOSE_PROVIDER="openai"',
							`export GOOSE_MODEL="${model}"`,
							`export OPENAI_HOST="${obotURL}"`,
							'export OPENAI_BASE_PATH="api/llm-proxy/v1/chat/completions"',
							`export OPENAI_API_KEY="${token}"`,
							'goose session'
						].join('\n')
					}
				];
			case 'cline':
				return [apiKeyBlock];
			case 'open-webui':
				return [
					apiKeyBlock,
					{
						title: m.models_connect_open_webui_docker(),
						language: 'bash' as const,
						code: [
							...(containerObotURL ? [`# ${m.models_connect_open_webui_docker_localhost()}`] : []),
							'docker run -d -p 3000:8080 \\',
							...(containerObotURL ? ['  --add-host=host.docker.internal:host-gateway \\'] : []),
							`  -e OPENAI_API_BASE_URL="${containerObotURL ? `${containerObotURL}/api/llm-proxy` : baseURL}/v1" \\`,
							`  -e OPENAI_API_KEY="${token}" \\`,
							'  -v open-webui:/app/backend/data --name open-webui --restart always \\',
							'  ghcr.io/open-webui/open-webui:main'
						].join('\n')
					}
				];
			default:
				return [];
		}
	});
</script>

<div class={twMerge('flex flex-col gap-3', klass)}>
	<div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5">
		{#each clients as client (client.id)}
			<button
				class="dark:bg-base-200 dark:border-base-400 bg-base-100 hover:border-primary flex items-start gap-3 rounded-lg border border-transparent p-4 text-left shadow-sm transition-colors"
				aria-labelledby="gateway-client-{client.id}-label"
				aria-describedby="gateway-client-{client.id}-description"
				onclick={() => open(client.id)}
			>
				<div
					class="bg-base-200 dark:bg-base-400 flex size-10 shrink-0 items-center justify-center rounded-md p-2"
				>
					{#if client.icon}
						<img
							src={client.icon}
							alt=""
							class={twMerge('size-full object-contain', client.monochrome && 'dark:invert')}
						/>
					{:else}
						<Terminal class="size-full" />
					{/if}
				</div>
				<div class="flex min-w-0 flex-col gap-0.5">
					<span id="gateway-client-{client.id}-label" class="text-sm font-semibold"
						>{client.label}</span
					>
					<span id="gateway-client-{client.id}-description" class="text-muted-content text-xs"
						>{client.description}</span
					>
				</div>
			</button>
		{/each}
	</div>
</div>

{#snippet modelPicker(key: PickerKey)}
	{#if showPicker(key)}
		<div class="flex flex-col gap-1">
			<span id="gateway-model-{key}-label" class="text-xs font-medium"
				>{m.models_connect_model_label()}</span
			>
			<Select
				id="gateway-model-{key}"
				options={pickerOptions[key]}
				selected={pickedModel(key)}
				searchInDropdown
				searchPlaceholder={m.models_gateway_search_models()}
				ariaLabelledby="gateway-model-{key}-label"
				ariaDescribedby={key === 'model' ? undefined : `gateway-model-${key}-description`}
				class="bg-base-200 dark:bg-base-100 min-h-9 py-1.5 text-sm shadow-none"
				onSelect={(option) => (picked = { ...picked, [key]: String(option.id) })}
			/>
			{#if key !== 'model'}
				<p id="gateway-model-{key}-description" class="text-muted-content text-xs">
					{key === 'claude-code-subscription'
						? m.models_connect_subscription_model_note()
						: m.models_connect_claude_code_model_note()}
				</p>
			{/if}
		</div>
	{/if}
{/snippet}

<ResponsiveDialog bind:this={dialog} title={selectedClient?.label} class="max-w-3xl">
	<div class="flex flex-col gap-3 pt-4">
		{#if selected !== 'claude-code'}
			{@render modelPicker('model')}
		{/if}
		{#if steps.length > 0}
			<ol class="flex list-decimal flex-col gap-1.5 pl-5 text-sm">
				{#each steps as step, i (`${selected}-step-${i}`)}
					<li>
						<span>{step.text}</span>
						{#if step.value}
							<span class="inline-flex max-w-full items-center gap-1 align-middle">
								<code class="bg-base-200 dark:bg-base-100 truncate rounded px-1.5 py-0.5 text-xs"
									>{step.value}</code
								>
								<CopyButton text={step.value} />
							</span>
						{/if}
						{#if step.note}
							<p class="text-muted-content text-xs">
								{step.note}
								{#if step.noteValue}
									<span class="inline-flex max-w-full items-center gap-1 align-middle">
										<code
											class="bg-base-200 dark:bg-base-100 truncate rounded px-1.5 py-0.5 text-xs"
											>{step.noteValue}</code
										>
										<CopyButton text={step.noteValue} />
									</span>
								{/if}
							</p>
						{/if}
					</li>
				{/each}
			</ol>
		{/if}
		{#each blocks as block, i (`${selected}-${i}`)}
			<LLMGatewayCodeBlock {block}>
				{#if block.picker}
					{@render modelPicker(block.picker)}
				{/if}
			</LLMGatewayCodeBlock>
		{/each}
	</div>
</ResponsiveDialog>
