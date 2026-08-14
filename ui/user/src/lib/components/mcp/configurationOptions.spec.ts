import {
	configurationSelectOptions,
	isMissingRequiredConfigurationField,
	selectedConfigurationOption
} from './configurationOptions';
import { describe, expect, it } from 'vitest';

const options = [
	{ name: 'United States', value: 'us', description: 'US endpoint' },
	{ name: 'Europe', value: 'eu', description: 'EU endpoint' }
];

describe('configuration options', () => {
	it('maps catalog options to select options without losing descriptions', () => {
		expect(configurationSelectOptions(options)).toEqual([
			{ ...options[0], id: 'us', label: 'United States' },
			{ ...options[1], id: 'eu', label: 'Europe' }
		]);
	});

	it('finds the selected option for description display', () => {
		expect(selectedConfigurationOption({ options, value: 'eu' })?.description).toBe('EU endpoint');
	});

	it('requires allowed selections while permitting an empty optional field', () => {
		expect(isMissingRequiredConfigurationField({ options, required: true, value: '' })).toBe(true);
		expect(isMissingRequiredConfigurationField({ options, required: true, value: 'stale' })).toBe(
			true
		);
		expect(isMissingRequiredConfigurationField({ options, required: true, value: 'us' })).toBe(
			false
		);
		expect(isMissingRequiredConfigurationField({ options, required: false, value: '' })).toBe(
			false
		);
		expect(isMissingRequiredConfigurationField({ options, required: false, value: 'stale' })).toBe(
			true
		);
	});
});
