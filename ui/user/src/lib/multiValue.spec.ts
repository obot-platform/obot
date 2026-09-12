import { parseMultiValue, serializeMultiValue } from './multiValue';
import { expect, test } from 'vitest';

test('round trips names without treating punctuation or whitespace as separators', () => {
	const values = ['Outlook, Calendar', 'A "quoted" \\ server', ' spaced ', '[server]'];
	expect(parseMultiValue(serializeMultiValue(values))).toEqual(values);
	expect(parseMultiValue(serializeMultiValue([]))).toEqual([]);
});

test('keeps existing comma-separated URLs and malformed JSON readable', () => {
	expect(parseMultiValue('one, two,,three')).toEqual(['one', 'two', 'three']);
	expect(parseMultiValue('[server]')).toEqual(['[server]']);
	expect(parseMultiValue('[123]')).toEqual(['[123]']);
	expect(parseMultiValue(null)).toEqual([]);
});
