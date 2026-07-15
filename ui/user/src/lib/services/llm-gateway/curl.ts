import type { RenderContext, SnippetBlock } from './types';

function loginSubstitution(obotURL: string): string {
	return `$(obot login --url ${obotURL} --scope llm --print-token)`;
}

/** Build a copy-pasteable curl example for the provider's chat/messages endpoint. */
export function renderCurlExample(ctx: RenderContext): SnippetBlock {
	const model = ctx.exampleModel ?? '<model-name>';

	if (
		ctx.provider.shortKey === 'anthropic' ||
		ctx.provider.shortKey === 'aws-bedrock-anthropic' ||
		ctx.provider.shortKey === 'aws-bedrock-api-key-anthropic'
	) {
		const body = JSON.stringify(
			{
				model,
				max_tokens: 1024,
				messages: [{ role: 'user', content: 'hello' }]
			},
			null,
			2
		);
		const apiKeyName = ctx.provider.shortKey === 'anthropic' ? 'ANTHROPIC_API_KEY' : 'OBOT_API_KEY';
		const baseURLName =
			ctx.provider.shortKey === 'anthropic' ? 'ANTHROPIC_BASE_URL' : 'OBOT_BASE_URL';
		const authHeader =
			ctx.provider.shortKey === 'anthropic'
				? `x-api-key: $${apiKeyName}`
				: `Authorization: Bearer $${apiKeyName}`;
		const code = [
			`export ${baseURLName}="${ctx.baseURL}"`,
			`export ${apiKeyName}="${loginSubstitution(ctx.obotURL)}"`,
			'',
			`curl $${baseURLName}/v1/messages \\`,
			`  -H "${authHeader}" \\`,
			'  -H "anthropic-version: 2023-06-01" \\',
			'  -H "content-type: application/json" \\',
			`  -d '${body}'`
		].join('\n');
		return { language: 'bash', code };
	}

	// OpenAI-compatible Responses API
	const body = JSON.stringify(
		{
			model,
			input: [{ role: 'user', content: 'hello' }]
		},
		null,
		2
	);
	const code = [
		`export OPENAI_BASE_URL="${ctx.baseURL}"`,
		`export OPENAI_API_KEY="${loginSubstitution(ctx.obotURL)}"`,
		'',
		'curl $OPENAI_BASE_URL/v1/responses \\',
		'  -H "Authorization: Bearer $OPENAI_API_KEY" \\',
		'  -H "Content-Type: application/json" \\',
		`  -d '${body}'`
	].join('\n');
	return { language: 'bash', code };
}
