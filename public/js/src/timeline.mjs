// SPDX-License-Identifier: FSL-1.1-MIT

// Completion-timeline entry builders. The machine state is an invisible HTML
// comment (`runbooks:done` / `runbooks:reopened`), so the rendered notes stay
// clean while the state stays parseable by the reviewer. See
// specs/runbooks/notes-timeline. Unit-tested in timeline.test.mjs.

// timelineEntry renders one timeline line. `kind` is "Step" or "Block"; `id` is
// the step number or the `s.b` block key; `label` is the title or the code label;
// `vars` names the inputs in play (ids only, never values).
export function timelineEntry({ kind, id, label, done, vars = [], at }) {
	const tail = done ? '' : ' (re-opened)';
	const varSuffix = done && vars.length ? ` (vars: ${vars.join(', ')})` : '';
	const state = done ? 'done' : 'reopened';
	return `- \`${at}\` ${kind} ${id}: ${label}${tail}${varSuffix} <!-- runbooks:${state} -->`;
}
