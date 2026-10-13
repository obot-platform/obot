import { Client } from '@modelcontextprotocol/sdk/client/index.js';
import type { AnySchema, SchemaOutput } from '@modelcontextprotocol/sdk/server/zod-compat.js';
import type { RequestOptions } from '@modelcontextprotocol/sdk/shared/protocol.js';
import {
	ListToolsResultSchema,
	ToolSchema,
	type Request
} from '@modelcontextprotocol/sdk/types.js';

export interface RejectedTesterTool {
	name?: string;
	index: number;
	cursor?: string;
	issues: unknown[];
}

// Filter at the result-parsing boundary, after LoggingTransport records the original message.
// Inheriting listTools keeps the SDK's output validators and task metadata intact.
export class TesterClient extends Client {
	rejectedTools: RejectedTesterTool[] = [];

	override request<T extends AnySchema>(
		request: Request,
		resultSchema: T,
		options?: RequestOptions
	): Promise<SchemaOutput<T>> {
		if (request.method !== 'tools/list' || (resultSchema as unknown) !== ListToolsResultSchema) {
			return super.request(request, resultSchema, options);
		}

		let index = 0;
		const schema = ListToolsResultSchema.extend({
			tools: ToolSchema.transform((tool) => {
				index += 1;
				return tool;
			})
				.optional()
				.catch((context) => {
					const input = context.input;
					this.rejectedTools.push({
						name:
							input &&
							typeof input === 'object' &&
							'name' in input &&
							typeof input.name === 'string'
								? input.name
								: undefined,
						index: index++,
						cursor: typeof request.params?.cursor === 'string' ? request.params.cursor : undefined,
						issues: context.error.issues
					});
					return undefined;
				})
				.array()
				.transform((tools) => tools.filter((tool) => tool !== undefined))
		});
		// This branch only replaces ListToolsResultSchema; the filtered output still conforms to it.
		return super.request(request, schema, options) as Promise<SchemaOutput<T>>;
	}
}
