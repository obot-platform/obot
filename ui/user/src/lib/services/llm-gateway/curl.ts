import type { RenderContext, SnippetBlock } from './types';

function loginSubstitution(obotURL: string): string {
	return `$(obot login --url ${obotURL} --print-token)`;
}

/** Build a copy-pasteable curl example for the provider's chat/messages endpoint. */
export function renderCurlExample(ctx: RenderContext): SnippetBlock {
	const model = ctx.exampleModel ?? '<model-name>';

	if (ctx.provider.shortKey === 'anthropic') {
		const body = JSON.stringify(
			{
				model,
				max_tokens: 1024,
				messages: [{ role: 'user', content: 'hello' }]
			},
			null,
			2
		);
		const code = [
			`export ANTHROPIC_BASE_URL="${ctx.baseURL}"`,
			`export ANTHROPIC_API_KEY="${loginSubstitution(ctx.obotURL)}"`,
			'',
			'curl $ANTHROPIC_BASE_URL/v1/messages \\',
			'  -H "x-api-key: $ANTHROPIC_API_KEY" \\',
			'  -H "anthropic-version: 2023-06-01" \\',
			'  -H "content-type: application/json" \\',
			`  -d '${body}'`
		].join('\n');
		return { language: 'bash', code };
	}

	// OpenAI (Responses API)
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
