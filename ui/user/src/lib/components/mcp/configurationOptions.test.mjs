import {
	configurationSelectOptions,
	isMissingRequiredConfigurationField,
	selectedConfigurationOption
} from './configurationOptions.ts';
import assert from 'node:assert/strict';
import test from 'node:test';

const options = [
	{ value: 'us', name: 'United States', description: 'PagerDuty US service region' },
	{ value: 'eu', name: 'Europe', description: 'PagerDuty EU service region' }
];

test('maps configuration options to Select items without losing descriptions', () => {
	assert.deepEqual(configurationSelectOptions(options), [
		{ ...options[0], id: 'us', label: 'United States' },
		{ ...options[1], id: 'eu', label: 'Europe' }
	]);
});

test('returns the selected option so its description can be displayed', () => {
	assert.equal(
		selectedConfigurationOption({ options, value: 'eu' })?.description,
		options[1].description
	);
	assert.equal(selectedConfigurationOption({ options, value: 'other' }), undefined);
});

test('requires a valid selection for required option fields', () => {
	assert.equal(isMissingRequiredConfigurationField({ options, required: true, value: '' }), true);
	assert.equal(
		isMissingRequiredConfigurationField({ options, required: true, value: 'other' }),
		true
	);
	assert.equal(
		isMissingRequiredConfigurationField({ options, required: true, value: 'us' }),
		false
	);
	assert.equal(isMissingRequiredConfigurationField({ options, required: false, value: '' }), false);
	assert.equal(
		isMissingRequiredConfigurationField({ options, required: true, value: '' }, false),
		false
	);
});

test('rejects a stale optional selection while allowing an empty one', () => {
	const field = { required: false, value: '', options };
	assert.equal(isMissingRequiredConfigurationField(field), false);
	field.value = 'removed';
	assert.equal(isMissingRequiredConfigurationField(field), true);
});
