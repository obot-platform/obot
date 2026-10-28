import type { DefaultModelAlias, Model } from '../user/types';
import { profileGrantedModels } from './profiles';
import { describe, expect, it } from 'vitest';

function model(id: string, targetModel: string, usage = 'llm'): Model {
	return { id, targetModel, usage } as Model;
}

const models = [
	model('m1-opus', 'anthropic/claude-opus-4-5'),
	model('m1-haiku', 'anthropic/claude-haiku-4-5'),
	model('m1-embed', 'openai/text-embedding-3', 'text-embedding')
];
const aliases = [{ alias: 'llm', model: 'm1-opus' }] as DefaultModelAlias[];

function granted(ids: string[]) {
	return profileGrantedModels({ models: ids.map((id) => ({ id })) }, models, aliases).map(
		(m) => m.id
	);
}

describe('profileGrantedModels', () => {
	it('grants every LLM model for a wildcard', () => {
		expect(granted(['*'])).toEqual(['m1-opus', 'm1-haiku']);
	});

	it('grants the models a profile names', () => {
		expect(granted(['m1-haiku'])).toEqual(['m1-haiku']);
	});

	it('grants the models a pattern matches', () => {
		expect(granted(['claude-haiku*'])).toEqual(['m1-haiku']);
	});

	it("grants an LLM alias's model", () => {
		expect(granted(['obot://llm'])).toEqual(['m1-opus']);
	});

	it('grants nothing without models', () => {
		expect(granted([])).toEqual([]);
		expect(profileGrantedModels(undefined, models)).toEqual([]);
	});

	it('never grants models that are not LLMs', () => {
		expect(granted(['m1-embed'])).toEqual([]);
	});
});
