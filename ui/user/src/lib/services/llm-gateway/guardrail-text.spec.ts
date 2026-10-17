import { getLocale, overwriteGetLocale } from '$lib/paraglide/runtime';
import {
	guardrailFieldDescription,
	guardrailFieldLabel,
	guardrailFindingMessage,
	guardrailTypeDescription,
	guardrailTypeLabel
} from './guardrail-text';
import { afterEach, describe, expect, it } from 'vitest';

const regex = {
	type: 'regex',
	label: 'Custom Regex',
	description:
		'Matches content against regular expressions, with ready-made patterns for common personal data.'
};

describe('guardrail text', () => {
	const restoreLocale = getLocale;
	afterEach(() => overwriteGetLocale(restoreLocale));

	it("shows the server's English in English", () => {
		expect(guardrailTypeLabel(regex)).toBe('Custom Regex');
		expect(guardrailFindingMessage('EMAIL detected')).toBe('EMAIL detected');
		expect(
			guardrailFieldDescription('regex', {
				name: 'patterns',
				description:
					'One pattern per entry, in RE2 syntax. Name a pattern with LABEL=pattern, e.g. EMPLOYEE_ID=E\\d{6}.'
			})
		).toBe(
			'One pattern per entry, in RE2 syntax. Name a pattern with LABEL=pattern, e.g. EMPLOYEE_ID=E\\d{6}.'
		);
	});

	it('translates known types, settings and findings', () => {
		overwriteGetLocale(() => 'ja');
		expect(guardrailTypeLabel(regex)).toBe('カスタム正規表現');
		expect(guardrailTypeDescription(regex)).not.toBe(regex.description);
		expect(guardrailFieldLabel('lakera', { name: 'apiKey', label: 'API key' })).toBe('API キー');
		expect(
			guardrailFieldDescription('regex', {
				name: 'patterns',
				description:
					'One pattern per entry, in RE2 syntax. Name a pattern with LABEL=pattern, e.g. EMPLOYEE_ID=E\\d{6}.'
			})
		).toContain('EMPLOYEE_ID=E\\d{6}');
		expect(guardrailFindingMessage('EMAIL detected')).toBe('EMAIL を検出');
		expect(guardrailFindingMessage('Violates the policy')).toBe('ポリシーに違反しています');
		expect(guardrailFindingMessage('Hate content detected (severity 4)')).toBe(
			'Hate のコンテンツを検出（重大度 4）'
		);
		expect(guardrailFindingMessage('VIOLENCE content (high confidence)')).toBe(
			'VIOLENCE のコンテンツ（信頼度: 高）'
		);
	});

	it("keeps the server's English for unknown types, changed text and unknown findings", () => {
		overwriteGetLocale(() => 'ko');
		expect(guardrailTypeLabel({ type: 'acme', label: 'Acme Guard' })).toBe('Acme Guard');
		expect(guardrailTypeLabel({ type: 'regex', label: 'Regular Expressions' })).toBe(
			'Regular Expressions'
		);
		expect(guardrailTypeLabel({ type: 'lakera', label: 'Lakera Guard (Check Point)' })).toBe(
			'Lakera Guard (Check Point)'
		);
		expect(guardrailFieldLabel('regex', { name: 'newField', label: 'New field' })).toBe(
			'New field'
		);
		expect(guardrailFindingMessage('Something unexpected')).toBe('Something unexpected');
	});
});
