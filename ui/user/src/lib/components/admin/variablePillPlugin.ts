import type { MilkdownPlugin } from '@milkdown/ctx';
import { $prose } from '@milkdown/kit/utils';
import { Plugin, PluginKey } from '@milkdown/prose/state';
import { Decoration, DecorationSet } from '@milkdown/prose/view';

const variablePillKey = new PluginKey('variablePill');

// Regex to match $VariableName (alphanumeric and underscores after $)
const VARIABLE_REGEX = /\$([a-zA-Z_][a-zA-Z0-9_]*)/g;

// Regex to match a variable followed by a separator (space, dash, slash)
const VARIABLE_WITH_SEPARATOR_REGEX = /\$([a-zA-Z_][a-zA-Z0-9_]*)([ \-/])/g;

export interface VariablePillOptions {
	onVariableAddition?: (variable: string) => void;
}

interface VariableMatch {
	variable: string;
	start: number;
	end: number;
}

function findVariables(doc: Parameters<typeof DecorationSet.create>[0]) {
	const decorations: Decoration[] = [];

	doc.descendants((node, pos) => {
		if (node.isText && node.text) {
			const text = node.text;
			let match;

			VARIABLE_REGEX.lastIndex = 0;
			while ((match = VARIABLE_REGEX.exec(text)) !== null) {
				const start = pos + match.index;
				const end = start + match[0].length;

				decorations.push(
					Decoration.inline(start, end, {
						class: 'variable-pill',
						'data-variable': match[1]
					})
				);
			}
		}
	});

	return DecorationSet.create(doc, decorations);
}

function findVariablesWithSeparators(doc: Parameters<typeof DecorationSet.create>[0]) {
	const variables: VariableMatch[] = [];

	doc.descendants((node, pos) => {
		if (node.isText && node.text) {
			const text = node.text;
			let match;

			VARIABLE_WITH_SEPARATOR_REGEX.lastIndex = 0;
			while ((match = VARIABLE_WITH_SEPARATOR_REGEX.exec(text)) !== null) {
				variables.push({
					variable: match[1],
					start: pos + match.index,
					end: pos + match.index + match[0].length
				});
			}
		}
	});

	return variables;
}

export function createVariablePillPlugin(options: VariablePillOptions = {}): MilkdownPlugin {
	return $prose(() => {
		let previousVariablesWithSeparators: VariableMatch[] = [];

		return new Plugin({
			key: variablePillKey,
			state: {
				init(_, { doc }) {
					previousVariablesWithSeparators = findVariablesWithSeparators(doc);
					return findVariables(doc);
				},
				apply(tr, decorations) {
					if (tr.docChanged) {
						const newVariablesWithSeparators = findVariablesWithSeparators(tr.doc);

						// Check for newly completed variables (variable + separator that wasn't there before)
						if (options.onVariableAddition) {
							for (const newVar of newVariablesWithSeparators) {
								// Check if this variable+separator combination is new
								const existedBefore = previousVariablesWithSeparators.some(
									(prev) =>
										prev.variable === newVar.variable &&
										prev.start === newVar.start &&
										prev.end === newVar.end
								);

								if (!existedBefore) {
									options.onVariableAddition(newVar.variable);
								}
							}
						}

						previousVariablesWithSeparators = newVariablesWithSeparators;
						return findVariables(tr.doc);
					}
					return decorations.map(tr.mapping, tr.doc);
				}
			},
			props: {
				decorations(state) {
					return this.getState(state);
				}
			}
		});
	});
}

// Default export for backwards compatibility (no callback)
export const variablePillPlugin: MilkdownPlugin = createVariablePillPlugin();
