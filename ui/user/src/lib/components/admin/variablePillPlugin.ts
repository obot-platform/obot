import type { MilkdownPlugin } from '@milkdown/ctx';
import { $prose } from '@milkdown/kit/utils';
import type { Node } from '@milkdown/prose/model';
import { Plugin, PluginKey } from '@milkdown/prose/state';
import { Decoration, DecorationSet } from '@milkdown/prose/view';

const variablePillKey = new PluginKey('variablePill');

// Regex to match $VariableName (alphanumeric and underscores after $)
const VARIABLE_REGEX = /\$([a-zA-Z_][a-zA-Z0-9_]*)/g;

// Separator characters that complete a variable (space, non-breaking space, dash, slash)
const SEPARATOR_CHARS = new Set([' ', '\u00A0', '-', '/']);

export interface VariablePillOptions {
	onVariableAddition?: (variable: string) => void;
	onVariableDeletion?: (variable: string) => void;
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

// Find all variables in the document (without requiring separators)
function findAllVariables(doc: Node): VariableMatch[] {
	const variables: VariableMatch[] = [];

	doc.descendants((node, pos) => {
		if (node.isText && node.text) {
			const text = node.text;
			let match;

			VARIABLE_REGEX.lastIndex = 0;
			while ((match = VARIABLE_REGEX.exec(text)) !== null) {
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

// Check if there's a separator character at the given position in the document
function hasSeparatorAt(doc: Node, pos: number): boolean {
	try {
		const $pos = doc.resolve(pos);
		const nodeAfter = $pos.nodeAfter;
		const nodeBefore = $pos.nodeBefore;
		const parent = $pos.parent;

		console.log(`hasSeparatorAt(${pos}) debug:`, {
			nodeAfter: nodeAfter?.isText ? nodeAfter.text : nodeAfter?.type?.name,
			nodeBefore: nodeBefore?.isText ? nodeBefore.text : nodeBefore?.type?.name,
			parentType: parent.type.name,
			parentOffset: $pos.parentOffset,
			parentTextContent: parent.isTextblock ? parent.textContent : 'N/A'
		});

		// Check if the node right after is text starting with a separator
		if (nodeAfter?.isText && nodeAfter.text) {
			const char = nodeAfter.text[0];
			console.log(`  nodeAfter first char: "${char}" (code: ${char.charCodeAt(0)})`);
			return SEPARATOR_CHARS.has(char);
		}

		// Check within parent textblock at the offset position
		if (parent.isTextblock) {
			const offset = $pos.parentOffset;
			const textContent = parent.textContent;
			if (offset < textContent.length) {
				const char = textContent[offset];
				console.log(`  parent textContent[${offset}]: "${char}" (code: ${char.charCodeAt(0)})`);
				return SEPARATOR_CHARS.has(char);
			} else {
				console.log(`  offset ${offset} >= textContent.length ${textContent.length}`);
			}
		}

		// No separator found - do NOT treat end-of-node as separator
		return false;
	} catch (e) {
		console.log(`hasSeparatorAt error:`, e);
		return false;
	}
}

// Check if the variable is now followed by a new block (Enter was pressed)
function isFollowedByNewBlock(doc: Node, pos: number): boolean {
	try {
		const $pos = doc.resolve(pos);

		// Check if we're at the end of a textblock and the next node is a block
		if ($pos.parentOffset === $pos.parent.content.size) {
			// We're at the end of the parent node
			const indexInGrandparent = $pos.index($pos.depth - 1);
			const grandparent = $pos.node($pos.depth - 1);

			// Check if there's a sibling block after this one
			if (indexInGrandparent + 1 < grandparent.childCount) {
				return true; // There's a block after this one
			}
		}

		return false;
	} catch {
		return false;
	}
}

export function createVariablePillPlugin(options: VariablePillOptions = {}): MilkdownPlugin {
	return $prose(() => {
		// Track variables from previous state
		let previousVariables: VariableMatch[] = [];
		const completedVariables = new Set<string>(); // "variable:start" keys that already triggered

		return new Plugin({
			key: variablePillKey,
			state: {
				init(_, { doc }) {
					previousVariables = findAllVariables(doc);
					// Mark any variables that already have separators as completed
					for (const v of previousVariables) {
						if (hasSeparatorAt(doc, v.end) || isFollowedByNewBlock(doc, v.end)) {
							completedVariables.add(`${v.variable}:${v.start}`);
						}
					}
					return findVariables(doc);
				},
				apply(tr, decorations) {
					if (tr.docChanged) {
						const newVariables = findAllVariables(tr.doc);

						if (options.onVariableAddition) {
							// For each variable that existed BEFORE this transaction,
							// check if it now has a separator (meaning user just typed one)
							for (const prevVar of previousVariables) {
								const key = `${prevVar.variable}:${prevVar.start}`;

								// Skip if already completed
								if (completedVariables.has(key)) {
									continue;
								}

								// Find the same variable in the new document
								const newVar = newVariables.find(
									(v) => v.variable === prevVar.variable && v.start === prevVar.start
								);

								// If the variable still exists, check if there's now a separator after it
								if (newVar) {
									const hasSep = hasSeparatorAt(tr.doc, newVar.end);
									const hasBlock = isFollowedByNewBlock(tr.doc, newVar.end);

									if (hasSep || hasBlock) {
										options.onVariableAddition(prevVar.variable);
										completedVariables.add(key);
									}
								}
							}
						}

						// Update previous variables for next transaction
						previousVariables = newVariables;

						// Clean up completed set - remove entries for variables that no longer exist
						const currentKeys = new Set(newVariables.map((v) => `${v.variable}:${v.start}`));
						for (const key of completedVariables) {
							if (!currentKeys.has(key)) {
								completedVariables.delete(key);
							}
						}

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
