// SPDX-License-Identifier: FSL-1.1-MIT

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { timelineEntry } from './timeline.mjs';

const at = '2026-02-11 14:32:07';

test('a completed step ends with the done comment', () => {
	assert.equal(
		timelineEntry({ kind: 'Step', id: '3', label: 'Restart replication', done: true, at }),
		'- `2026-02-11 14:32:07` Step 3: Restart replication <!-- runbooks:done -->',
	);
});

test('a re-opened step carries the reopened comment', () => {
	assert.equal(
		timelineEntry({ kind: 'Step', id: '3', label: 'Restart replication', done: false, at }),
		'- `2026-02-11 14:32:07` Step 3: Restart replication (re-opened) <!-- runbooks:reopened -->',
	);
});

test('filled inputs are named by id, and only on completion', () => {
	assert.equal(
		timelineEntry({ kind: 'Step', id: '4', label: 'Verify', done: true, vars: ['HOST', 'TOKEN'], at }),
		'- `2026-02-11 14:32:07` Step 4: Verify (vars: HOST, TOKEN) <!-- runbooks:done -->',
	);
	assert.ok(!timelineEntry({ kind: 'Step', id: '4', label: 'Verify', done: false, vars: ['HOST'], at }).includes('vars:'));
});

test('a block entry uses the s.b key', () => {
	assert.equal(
		timelineEntry({ kind: 'Block', id: '3.2', label: 'mysql> START SLAVE', done: true, at }),
		'- `2026-02-11 14:32:07` Block 3.2: mysql> START SLAVE <!-- runbooks:done -->',
	);
});

test('the visible line carries no marker glyphs', () => {
	const line = timelineEntry({ kind: 'Step', id: '1', label: 'x', done: true, at });
	assert.ok(!/[✅☑↩]/.test(line), `unexpected glyph in ${line}`);
});
