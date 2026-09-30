import {
	convertSubjectsToTableData,
	resolveSubjectFromGroup,
	resolveSubjectPickerById
} from './subjectResolver';
import { describe, expect, it } from 'vitest';

describe('resolveSubjectFromGroup', () => {
	it('stores all users as a selector', () => {
		expect(resolveSubjectFromGroup({ id: '*', name: 'All Obot Users' })).toEqual({
			type: 'selector',
			id: '*'
		});
	});

	it('stores the Obot Admin picker row as an obot group', () => {
		expect(resolveSubjectFromGroup({ id: 'obot-admin', name: 'Obot Admin' })).toEqual({
			type: 'obotGroup',
			id: 'admin'
		});
	});

	it('stores a directory group whose id is admin as a group', () => {
		expect(resolveSubjectFromGroup({ id: 'admin', name: 'Admin' })).toEqual({
			type: 'group',
			id: 'admin'
		});
	});

	it('uses a synthetic picker id for Obot Admin', () => {
		expect(resolveSubjectPickerById({ type: 'obotGroup', id: 'admin' })).toBe('obot-admin');
		expect(resolveSubjectPickerById({ type: 'group', id: 'admin' })).toBe('admin');
	});

	it('stores a directory group as a group', () => {
		expect(resolveSubjectFromGroup({ id: 'entra/engineering', name: 'Engineering' })).toEqual({
			type: 'group',
			id: 'entra/engineering'
		});
	});
});

describe('convertSubjectsToTableData', () => {
	it('labels an obot group without a directory lookup', () => {
		expect(convertSubjectsToTableData([{ type: 'obotGroup', id: 'admin' }], [], [])).toEqual([
			{ id: 'obot-admin', displayName: 'Obot Admin', type: 'Group' }
		]);
	});

	it('uses the directory name for a group subject', () => {
		expect(
			convertSubjectsToTableData(
				[{ type: 'group', id: 'entra/engineering' }],
				[],
				[{ id: 'entra/engineering', name: 'Engineering' }]
			)
		).toEqual([{ id: 'entra/engineering', displayName: 'Engineering', type: 'Group' }]);
	});
});
