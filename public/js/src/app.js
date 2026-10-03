// Variable substitution + copy-to-clipboard for runbook pages

import { decodeBase64URL, encodeBase64URL } from './base64url.mjs';

const vars = {};
const secretVars = new Set();

function escapeHtml(str) {
	return str
		.replace(/&/g, '&amp;')
		.replace(/</g, '&lt;')
		.replace(/>/g, '&gt;')
		.replace(/"/g, '&quot;');
}

// setHTML replaces an element's children with parsed HTML. A DOMParser never
// runs scripts, and every caller passes escaped or sanitized markup, so this is
// the single place an HTML string becomes DOM (rather than a bare innerHTML).
function setHTML(el, html) {
	const parsed = new DOMParser().parseFromString(html, 'text/html');
	el.replaceChildren(...parsed.body.childNodes);
}

// Substitute {{VAR_NAME}} tokens in already-highlighted HTML.
// hljs does not escape { } so tokens survive highlighting intact.
function substituteVars(html) {
	return html.replace(/\{\{([A-Z_]+)\}\}/g, (_, name) => {
		const val = vars[name];
		if (!val) return `<span class="var-placeholder">&lt;${name}&gt;</span>`;
		if (secretVars.has(name)) return `<span class="var-secret">••••••••</span>`;
		return `<span class="var-filled">${escapeHtml(val)}</span>`;
	});
}

function renderBlock(template, lang) {
	const hljs = window.hljs;
	let highlighted;
	try {
		if (lang && hljs.getLanguage(lang)) {
			highlighted = hljs.highlight(template, {language: lang}).value;
		} else {
			highlighted = hljs.highlightAuto(template, ['sql', 'bash', 'shell', 'plaintext']).value;
		}
	} catch (_) {
		highlighted = escapeHtml(template);
	}
	return substituteVars(highlighted);
}

function updateAll() {
	document.querySelectorAll('[data-template]').forEach(block => {
		const code = block.querySelector('code');
		if (code) setHTML(code, renderBlock(block.dataset.template, block.dataset.lang || ''));
	});
}

// Wire up variable inputs
document.querySelectorAll('[data-var]').forEach(input => {
	if (input.type === 'password') secretVars.add(input.dataset.var);
	input.addEventListener('input', () => {
		vars[input.dataset.var] = input.value;
		updateAll();
	});
});

// Initialise display with placeholders
updateAll();

// Inputs fold — collapse the variables panel to a one-line summary. The server
// folds by count (more than two) so the first paint is right; a stored
// `runbooks-vars` preference overrides it. The summary's "N set" is live.
const varsPanel = document.querySelector('[data-vars-panel]');
if (varsPanel) {
	const varsToggle = varsPanel.querySelector('[data-vars-toggle]');
	const varsSet = varsPanel.querySelector('[data-vars-set]');
	const varsInputs = [...varsPanel.querySelectorAll('[data-var]')];

	const applyVarsFold = folded => {
		varsPanel.classList.toggle('is-folded', folded);
		document.documentElement.dataset.vars = folded ? 'folded' : 'expanded';
		varsToggle.setAttribute('aria-expanded', String(!folded));
	};

	const updateVarsSet = () => {
		const set = varsInputs.filter(i => i.value.trim() !== '').length;
		varsSet.textContent = set ? `${set} set` : '';
	};

	varsInputs.forEach(i => i.addEventListener('input', updateVarsSet));
	updateVarsSet();

	const stored = localStorage.getItem('runbooks-vars');
	applyVarsFold(stored ? stored === 'folded' : varsPanel.classList.contains('is-folded'));

	varsToggle.addEventListener('click', () => {
		const folded = !varsPanel.classList.contains('is-folded');
		localStorage.setItem('runbooks-vars', folded ? 'folded' : 'expanded');
		applyVarsFold(folded);
	});
}

// Copy-to-clipboard
document.addEventListener('click', e => {
	const btn = e.target.closest('.copy-btn');
	if (!btn) return;
	const block = btn.closest('[data-template]');
	if (!block) return;

	// Build copy text from the raw template so secrets copy as plaintext, not bullets.
	const text = block.dataset.template.replace(/\{\{([A-Z_]+)\}\}/g, (_, name) =>
		vars[name] ?? `<${name}>`
	);
	navigator.clipboard.writeText(text).then(() => {
		btn.textContent = 'Copied';
		btn.classList.add('copied');
		setTimeout(() => {
			btn.textContent = 'Copy';
			btn.classList.remove('copied');
		}, 1500);
	});
});

// Step + sub-step completion, persisted in URL hash.
// Format: #done=s1,s3,b4-1,b4-2
//   s<n>   = step n (1-based, non-rollback)
//   b<s>-<b> = labeled code block b within step s

const stepCards  = () => [...document.querySelectorAll('.step-card:not(.rollback-card)')];
const codeGroups = () => stepCards().flatMap((card, si) =>
	[...card.querySelectorAll('.code-group')].map((group, bi) => ({
		group, key: `b${si + 1}-${bi + 1}`,
	}))
);

// A step follows its own blocks: once every labeled block in it is ticked, the
// step ticks itself. A manually ticked step is left alone (no roll-back down).
function rollUpStep(card, record) {
	if (!card || card.classList.contains('rollback-card') || card.classList.contains('done')) return;
	const groups = [...card.querySelectorAll('.code-group')];
	if (groups.length === 0 || !groups.every(g => g.classList.contains('done'))) return;
	card.classList.add('done');
	if (record) recordTimeline(stepTimelineEntry(card, true));
}

function syncHash() {
	const parts = [];
	stepCards().forEach((card, i) => {
		if (card.classList.contains('done')) parts.push(`s${i + 1}`);
	});
	codeGroups().forEach(({group, key}) => {
		if (group.classList.contains('done')) parts.push(key);
	});
	const base = window.location.pathname + window.location.search;
	history.replaceState(null, '', parts.length ? `${base}#done=${parts.join(',')}` : base);
}

function loadHash() {
	const m = window.location.hash.match(/^#done=([\w,.-]+)$/);
	if (!m) return;
	const tokens = new Set(m[1].split(','));
	stepCards().forEach((card, i) => {
		if (tokens.has(`s${i + 1}`)) card.classList.add('done');
	});
	codeGroups().forEach(({group, key}) => {
		if (tokens.has(key)) group.classList.add('done');
	});
}

loadHash();
// A shared URL whose blocks are all done reads as a done step on load, without
// adding a timeline entry.
stepCards().forEach(card => rollUpStep(card, false));

document.addEventListener('click', e => {
	const stepBtn = e.target.closest('.step-check');
	if (stepBtn) {
		const card = stepBtn.closest('.step-card');
		if (card) {
			recordTimeline(stepTimelineEntry(card, card.classList.toggle('done')));
			syncHash();
		}
		return;
	}
	const blockBtn = e.target.closest('.block-check');
	if (blockBtn) {
		const group = blockBtn.closest('.code-group');
		if (group) {
			const done = group.classList.toggle('done');
			recordTimeline(blockTimelineEntry(group, done));
			rollUpStep(group.closest('.step-card'), true);
			syncHash();
		}
	}
});

// Hint popup — singleton panel, one per page
const hintPopup = document.createElement('div');
hintPopup.className = 'hint-popup';
hintPopup.innerHTML = '<p class="hint-popup-label">How to find this value</p><pre></pre>';
document.body.appendChild(hintPopup);

const hintPre = hintPopup.querySelector('pre');
let activeHintBtn = null;

function openHint(btn) {
	if (activeHintBtn) activeHintBtn.classList.remove('active');
	activeHintBtn = btn;
	btn.classList.add('active');

	hintPre.textContent = btn.dataset.hint;

	// Position popup to the right of the rail, vertically near the button.
	// Popup uses visibility:hidden so offsetHeight is always valid.
	const railWidth = document.querySelector('.rail').offsetWidth;
	const r = btn.getBoundingClientRect();
	const popupH = hintPopup.offsetHeight;
	const top = Math.max(8, Math.min(r.top, window.innerHeight - popupH - 8));

	hintPopup.style.left = (railWidth + 8) + 'px';
	hintPopup.style.top = top + 'px';
	hintPopup.classList.add('open');
}

function closeHint() {
	hintPopup.classList.remove('open');
	if (activeHintBtn) activeHintBtn.classList.remove('active');
	activeHintBtn = null;
}

document.querySelectorAll('.hint-btn').forEach(btn => {
	btn.addEventListener('click', e => {
		e.stopPropagation();
		if (activeHintBtn === btn) {
			closeHint();
		} else {
			openHint(btn);
		}
	});
});

document.addEventListener('click', e => {
	if (activeHintBtn && !hintPopup.contains(e.target)) closeHint();
});

document.addEventListener('keydown', e => {
	if (e.key === 'Escape') closeHint();
});

// Notes panel — auto-saved to localStorage per runbook page
const notesKey = 'runbooks-notes' + location.pathname;
const notesImgKey = 'runbooks-notes-imgs' + location.pathname;
const notesArea = document.querySelector('.notes-textarea');
const notesPreview = document.querySelector('.notes-preview');
const notesTabs = document.querySelectorAll('.notes-tab');
const notesClear = document.querySelector('.notes-clear');
const notesPrint = document.querySelector('.notes-print');
const notesExport = document.querySelector('.notes-export');

// Server-rendered runtime config: record layout + whether sync wants a bearer.
const pageConfig = (() => {
	const el = document.getElementById('page-config');
	if (!el) return {};
	try {
		return JSON.parse(el.textContent) || {};
	} catch (_) {
		return {};
	}
})();

// Expand ![alt][img-N] tokens → ![alt](data:...) before rendering/export
function expandNoteImgs(md) {
	const imgs = JSON.parse(localStorage.getItem(notesImgKey) || '{}');
	return md.replace(/!\[([^\]]*)\]\[img-(\d+)\]/g, (match, alt, n) => {
		const src = imgs[`img-${n}`];
		return src ? `![${alt}](${src})` : match;
	});
}

// Insert text through execCommand so the browser's native undo stack still
// reverses toolbar and paste edits.
function insertNotesText(text, start, end) {
	notesArea.focus();
	notesArea.setSelectionRange(start, end);
	document.execCommand('insertText', false, text);
}

// Store an image as an img-N token and drop its markdown reference at the
// caret. Shared by clipboard paste and the Screenshot button.
function insertNoteImage(file) {
	const reader = new FileReader();
	reader.onload = ev => {
		const imgs = JSON.parse(localStorage.getItem(notesImgKey) || '{}');
		const n = Object.keys(imgs).filter(k => k.startsWith('img-')).length + 1;
		const id = `img-${n}`;
		imgs[id] = ev.target.result;
		localStorage.setItem(notesImgKey, JSON.stringify(imgs));
		const md = `\n![screenshot][${id}]\n`;
		const start = notesArea.selectionStart;
		insertNotesText(md, start, notesArea.selectionEnd);
		notesArea.setSelectionRange(start + md.length, start + md.length);
	};
	reader.readAsDataURL(file);
}

if (notesArea) {
	notesArea.value = localStorage.getItem(notesKey) || '';
	notesArea.addEventListener('input', () => localStorage.setItem(notesKey, notesArea.value));

	notesArea.addEventListener('paste', e => {
		// A URL pasted over a selection becomes a link around that selection.
		const text = (e.clipboardData?.getData('text/plain') || '').trim();
		const selStart = notesArea.selectionStart;
		const selEnd = notesArea.selectionEnd;
		if (selEnd > selStart && /^https?:\/\/\S+$/i.test(text)) {
			e.preventDefault();
			insertNotesText(`[${notesArea.value.slice(selStart, selEnd)}](${text})`, selStart, selEnd);
			return;
		}
		const imgItem = Array.from(e.clipboardData?.items || []).find(i => i.type.startsWith('image/'));
		if (!imgItem) return;
		e.preventDefault();
		insertNoteImage(imgItem.getAsFile());
	});
}

// Completion timeline — each tick appends a timestamped line to the notes body.
// Entries are plain markdown, so Preview, Print and Export pick them up for free.
const recordKey = 'runbooks-record-timeline';
const recordCheck = document.querySelector('.notes-record-check');
let recordEnabled = localStorage.getItem(recordKey) !== 'off';
if (recordCheck) {
	recordCheck.checked = recordEnabled;
	recordCheck.addEventListener('change', () => {
		recordEnabled = recordCheck.checked;
		localStorage.setItem(recordKey, recordEnabled ? 'on' : 'off');
	});
}

function localTimestamp(d) {
	const p = n => String(n).padStart(2, '0');
	return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}`;
}

// Filled inputs are named by id only — values may be secrets and the notes get exported.
function filledVarSuffix() {
	const ids = Object.keys(vars).filter(name => vars[name]);
	return ids.length ? ` (vars: ${ids.join(', ')})` : '';
}

function stepTimelineEntry(card, done) {
	const num = (card.querySelector('.step-num')?.textContent || '').trim();
	const title = (card.querySelector('.step-title')?.textContent || '').trim();
	const tail = done ? '' : ' (re-opened)';
	return `- \`${localTimestamp(new Date())}\` ${done ? '✅' : '↩'} Step ${num} — ${title}${tail}${done ? filledVarSuffix() : ''}`;
}

function blockTimelineEntry(group, done) {
	const card = group.closest('.step-card');
	const si = stepCards().indexOf(card) + 1;
	const bi = [...card.querySelectorAll('.code-group')].indexOf(group) + 1;
	const label = (group.querySelector('.code-label')?.textContent || '').trim();
	const tail = done ? '' : ' (re-opened)';
	return `- \`${localTimestamp(new Date())}\` ${done ? '☑' : '↩'} Block ${si}.${bi} — ${label}${tail}${done ? filledVarSuffix() : ''}`;
}

const timelineLine = /^- `\d{4}-\d{2}-\d{2} /;

function recordTimeline(entry) {
	if (!notesArea || !recordEnabled) return;
	const body = notesArea.value;
	const lastLine = body.split('\n').filter(l => l.trim() !== '').pop() || '';
	const sep = body === '' ? '' : body.replace(/\n*$/, '') + (timelineLine.test(lastLine) ? '\n' : '\n\n');
	const next = sep + entry;

	const selStart = notesArea.selectionStart;
	const selEnd = notesArea.selectionEnd;
	notesArea.value = next;
	notesArea.selectionStart = selStart;
	notesArea.selectionEnd = selEnd;
	localStorage.setItem(notesKey, next);

	if (notesPreview && !notesPreview.hidden) {
		renderNotesPreview(next);
	}
}

// Render the notes markdown, making external links open in a new tab so
// following a reference does not navigate away from an in-progress run. The
// rendered HTML is untrusted (notes are pasted and shared), so it is sanitized
// to a strict allowlist before it becomes DOM.
function renderNotesPreview(md) {
	setHTML(notesPreview, window.DOMPurify.sanitize(window.marked.parse(expandNoteImgs(md))));
	notesPreview.querySelectorAll('a[href]').forEach(a => {
		a.target = '_blank';
		a.rel = 'noopener noreferrer';
	});
}

function setNotesMode(mode) {
	notesTabs.forEach(b => b.classList.toggle('active', b.dataset.mode === mode));
	document.querySelector('.notes-panel')?.setAttribute('data-mode', mode);
	if (mode === 'preview') {
		renderNotesPreview(notesArea.value || '');
		notesPreview.hidden = false;
		notesArea.hidden = true;
	} else {
		notesArea.hidden = false;
		notesPreview.hidden = true;
		notesArea.focus();
	}
}

notesTabs.forEach(btn => btn.addEventListener('click', () => setNotesMode(btn.dataset.mode)));

// Composer toolbar: inline buttons wrap the selection, block buttons prefix its
// lines. A second press removes the marker.
function notesWrap(before, after, placeholder) {
	const start = notesArea.selectionStart;
	const end = notesArea.selectionEnd;
	const value = notesArea.value;
	const selected = value.slice(start, end);
	// Markers sitting just outside the selection (the post-wrap state).
	if (value.slice(start - before.length, start) === before && value.slice(end, end + after.length) === after) {
		insertNotesText(selected, start - before.length, end + after.length);
		notesArea.setSelectionRange(start - before.length, end - before.length);
		return;
	}
	if (selected.startsWith(before) && selected.endsWith(after) && selected.length >= before.length + after.length) {
		const stripped = selected.slice(before.length, selected.length - after.length);
		insertNotesText(stripped, start, end);
		notesArea.setSelectionRange(start, start + stripped.length);
		return;
	}
	const body = selected || placeholder;
	insertNotesText(before + body + after, start, end);
	notesArea.setSelectionRange(start + before.length, start + before.length + body.length);
}

function notesLink() {
	const start = notesArea.selectionStart;
	const end = notesArea.selectionEnd;
	const body = notesArea.value.slice(start, end) || 'text';
	insertNotesText(`[${body}](url)`, start, end);
	const urlStart = start + body.length + 3;
	notesArea.setSelectionRange(urlStart, urlStart + 3);
}

function notesPrefix(prefix) {
	const value = notesArea.value;
	const start = notesArea.selectionStart;
	const end = notesArea.selectionEnd;
	const lineStart = value.lastIndexOf('\n', start - 1) + 1;
	let lineEnd = value.indexOf('\n', end);
	if (lineEnd === -1) lineEnd = value.length;
	const lines = value.slice(lineStart, lineEnd).split('\n');
	const all = lines.every(line => line.startsWith(prefix));
	const next = lines.map(line => (all ? line.slice(prefix.length) : prefix + line)).join('\n');
	insertNotesText(next, lineStart, lineEnd);
	notesArea.setSelectionRange(lineStart, lineStart + next.length);
}

document.querySelectorAll('[data-notes-tool]').forEach(btn => {
	btn.addEventListener('click', () => {
		switch (btn.dataset.notesTool) {
			case 'bold':
				notesWrap('**', '**', 'bold');
				break;
			case 'italic':
				notesWrap('*', '*', 'italic');
				break;
			case 'code':
				notesWrap('`', '`', 'code');
				break;
			case 'link':
				notesLink();
				break;
			case 'ul':
				notesPrefix('- ');
				break;
			case 'ol':
				notesPrefix('1. ');
				break;
			case 'heading':
				notesPrefix('## ');
				break;
			case 'quote':
				notesPrefix('> ');
				break;
		}
	});
});

// Screenshot button: the file-picker twin of the paste handler.
const notesImageInput = document.createElement('input');
notesImageInput.type = 'file';
notesImageInput.accept = 'image/*';
notesImageInput.hidden = true;
document.body.appendChild(notesImageInput);
document.querySelector('[data-notes-tool="image"]')?.addEventListener('click', () => notesImageInput.click());
notesImageInput.addEventListener('change', () => {
	const file = notesImageInput.files[0];
	if (file) insertNoteImage(file);
	notesImageInput.value = '';
});

// Overflow menu and the syntax cheatsheet: non-modal, closed by outside click
// or Esc, anchored in the notes header.
const notesMenuBtn = document.querySelector('[data-notes-menu]');
const notesMenu = document.querySelector('.notes-menu');
const notesHelpBtn = document.querySelector('[data-notes-help]');
const notesCheatsheet = document.querySelector('[data-notes-cheatsheet]');

function setNotesMenu(open) {
	if (!notesMenu) return;
	notesMenu.hidden = !open;
	notesMenuBtn.setAttribute('aria-expanded', String(open));
}

function setNotesHelp(open) {
	if (!notesCheatsheet) return;
	notesCheatsheet.hidden = !open;
	notesHelpBtn.setAttribute('aria-expanded', String(open));
}

notesMenuBtn?.addEventListener('click', e => {
	e.stopPropagation();
	setNotesHelp(false);
	setNotesMenu(notesMenu.hidden);
});
notesHelpBtn?.addEventListener('click', e => {
	e.stopPropagation();
	setNotesMenu(false);
	setNotesHelp(notesCheatsheet.hidden);
});
notesMenu?.addEventListener('click', () => setNotesMenu(false));
document.addEventListener('click', e => {
	if (notesMenu && !notesMenu.hidden && !notesMenu.contains(e.target) && e.target !== notesMenuBtn) setNotesMenu(false);
	if (notesCheatsheet && !notesCheatsheet.hidden && !notesCheatsheet.contains(e.target) && e.target !== notesHelpBtn)
		setNotesHelp(false);
});
document.addEventListener('keydown', e => {
	if (e.key === 'Escape') {
		setNotesMenu(false);
		setNotesHelp(false);
	}
});

if (notesClear) {
	notesClear.addEventListener('click', async () => {
		const { confirmed } = await showDialog({
			title: 'Clear everything?',
			message:
				'Clear the notes, pasted screenshots, the timeline, and all completed steps? This cannot be undone.',
			confirmLabel: 'Clear',
			danger: true,
		});
		if (!confirmed) return;
		notesArea.value = '';
		localStorage.removeItem(notesKey);
		localStorage.removeItem(notesImgKey);
		stepCards().forEach(card => card.classList.remove('done'));
		codeGroups().forEach(({group}) => group.classList.remove('done'));
		syncHash();
		setNotesMode('edit');
	});
}

if (notesPrint) {
	notesPrint.addEventListener('click', () => {
		setNotesMode('preview');
		requestAnimationFrame(() => window.print());
	});
}

// Record path for the exported ZIP: <base>/<YYYY-MM-DD>-<slug>/. Mirrors the
// server's git-sync layout so a manual commit produces the same artifact, which
// is what makes Export the no-repo-credential path.
function exportRecordDir() {
	const base = (pageConfig.recordsBasePath || 'runs').replace(/\/+$/, '');
	const date = new Date().toISOString().slice(0, 10); // UTC, matches the server
	const slug = location.pathname.slice(1).replace(/\//g, '-') || 'runbook';
	const name = `${date}-${slug}`;
	return { dir: base ? `${base}/${name}` : name, name };
}

if (notesExport) {
	notesExport.addEventListener('click', async () => {
		const imgs = JSON.parse(localStorage.getItem(notesImgKey) || '{}');
		const mimeToExt = { jpeg: 'jpg', jpg: 'jpg', png: 'png', gif: 'gif', webp: 'webp' };
		const { dir, name } = exportRecordDir();

		const zip = new window.JSZip();

		// Add each image and build markdown with local filenames
		const md = notesArea.value.replace(/!\[([^\]]*)\]\[img-(\d+)\]/g, (match, alt, n) => {
			const src = imgs[`img-${n}`];
			if (!src) return match;
			const raw = (src.match(/^data:image\/(\w+)/) || ['', 'png'])[1];
			const ext = mimeToExt[raw] || raw;
			const filename = `img-${n}.${ext}`;
			zip.file(`${dir}/${filename}`, src.split(',')[1], { base64: true });
			return `![${alt}](${filename})`;
		});

		zip.file(`${dir}/notes.md`, md);

		const sourceEl = document.getElementById('runbook-source');
		if (sourceEl) {
			zip.file(`${dir}/runbook.md`, JSON.parse(sourceEl.textContent));
		}

		const blob = await zip.generateAsync({ type: 'blob' });
		const a = document.createElement('a');
		a.href = URL.createObjectURL(blob);
		a.download = `${name}.zip`;
		a.click();
		URL.revokeObjectURL(a.href);
	});
}

// Notes visibility is shared by the header toggle, the narrow drawer and the
// drag-to-resize handler below. setNotesGlyph swaps the button state; setNotesShown
// also drives the wide-layout column.
function setNotesGlyph(shown) {
	const t = document.querySelector('[data-notes-toggle]');
	if (!t) return;
	t.setAttribute('aria-pressed', String(shown));
	t.setAttribute('aria-label', shown ? 'Hide notes' : 'Show notes');
	const g = t.querySelector('.notes-toggle-glyph');
	if (g) g.textContent = shown ? '▸' : '◂';
}

function setNotesShown(shown) {
	document.body.classList.toggle('notes-hidden', !shown);
	setNotesGlyph(shown);
}

// Notes panel drag-to-resize. The width is state (a custom property) rather
// than a hard-coded grid: hiding the panel must drop the column entirely, and
// an inline grid-template-columns would outlive the hidden class. Dragging the
// edge in past the usable minimum dismisses the panel instead of clipping its
// wrapped toolbar.
const notesHandle = document.querySelector('.notes-resize');
const notesWidthKey = 'runbooks-notes-width';
const notesResizeMin = 340;
const notesResizeMax = 640;
const clampNotesWidth = w => Math.min(notesResizeMax, Math.max(notesResizeMin, Math.round(w)));
const savedNotesWidth = parseInt(localStorage.getItem(notesWidthKey) || '', 10);
if (savedNotesWidth >= notesResizeMin) document.body.style.setProperty('--notes-width', clampNotesWidth(savedNotesWidth) + 'px');
if (notesHandle) {
	notesHandle.addEventListener('mousedown', e => {
		const startX = e.clientX;
		const startW = document.querySelector('.notes-panel').offsetWidth;
		document.body.classList.add('resizing-notes');

		const onMove = ev => {
			const w = startW + (startX - ev.clientX);
			if (w >= notesResizeMin) {
				// Inside the usable range the panel follows the cursor.
				document.body.classList.remove('notes-dismiss-armed');
				document.body.style.setProperty('--notes-width', clampNotesWidth(w) + 'px');
				return;
			}
			// Past the floor it pins at the minimum and arms the dismissal.
			document.body.style.setProperty('--notes-width', notesResizeMin + 'px');
			document.body.classList.add('notes-dismiss-armed');
		};
		const onUp = () => {
			const dismiss = document.body.classList.contains('notes-dismiss-armed');
			document.body.classList.remove('resizing-notes', 'notes-dismiss-armed');
			document.removeEventListener('mousemove', onMove);
			document.removeEventListener('mouseup', onUp);
			if (dismiss) {
				setNotesShown(false);
				localStorage.setItem('runbooks-notes', 'off');
				return;
			}
			localStorage.setItem('runbooks-notes', 'on');
			localStorage.setItem(notesWidthKey, String(Math.round(document.querySelector('.notes-panel').offsetWidth)));
		};
		document.addEventListener('mousemove', onMove);
		document.addEventListener('mouseup', onUp);
		e.preventDefault();
	});
}

// Modal dialog (native <dialog>) — replaces window.confirm()/prompt(). Resolves
// { confirmed, value }. `input` adds a text field; `danger` styles the primary
// action as destructive. The browser supplies the backdrop, focus trap and Esc.
function showDialog({ title, message, confirmLabel = 'OK', cancelLabel = 'Cancel', danger = false, input = null }) {
	return new Promise(resolve => {
		const dialog = document.createElement('dialog');
		dialog.className = 'dialog';

		const header = document.createElement('div');
		header.className = 'dialog-header';
		header.textContent = title;

		const body = document.createElement('div');
		body.className = 'dialog-body';
		const text = document.createElement('p');
		text.textContent = message;
		body.appendChild(text);

		let field = null;
		if (input) {
			field = document.createElement('input');
			field.className = 'dialog-input';
			field.type = input.type || 'text';
			if (input.placeholder) field.placeholder = input.placeholder;
			body.appendChild(field);
		}

		const footer = document.createElement('div');
		footer.className = 'dialog-footer';
		const cancel = document.createElement('button');
		cancel.type = 'button';
		cancel.className = 'btn btn-ghost';
		cancel.textContent = cancelLabel;
		const confirm = document.createElement('button');
		confirm.type = 'button';
		confirm.className = danger ? 'btn btn-danger' : 'btn btn-primary';
		confirm.textContent = confirmLabel;
		footer.append(cancel, confirm);

		dialog.append(header, body, footer);
		document.body.appendChild(dialog);

		const finish = confirmed => {
			const value = field ? field.value : undefined;
			dialog.close();
			dialog.remove();
			resolve({ confirmed, value });
		};
		cancel.addEventListener('click', () => finish(false));
		confirm.addEventListener('click', () => finish(true));
		dialog.addEventListener('cancel', event => {
			event.preventDefault();
			finish(false);
		});
		if (field) {
			field.addEventListener('keydown', event => {
				if (event.key === 'Enter') finish(true);
			});
		}

		dialog.showModal();
		(field || confirm).focus();
	});
}

// Declarative dialog open/close (server-rendered dialogs): a trigger carries
// data-dialog-open="id"; anything with data-dialog-close closes its dialog.
document.querySelectorAll('[data-dialog-open]').forEach(trigger => {
	trigger.addEventListener('click', () => {
		const dialog = document.getElementById(trigger.dataset.dialogOpen);
		if (dialog) dialog.showModal();
	});
});
document.querySelectorAll('[data-dialog-close]').forEach(trigger => {
	trigger.addEventListener('click', () => trigger.closest('dialog')?.close());
});

// Theme switcher
const html = document.documentElement;

function setTheme(t) {
	localStorage.setItem('runbooks-theme', t);
	const dark = t === 'dark' || (t === 'system' && window.matchMedia('(prefers-color-scheme: dark)').matches);
	html.dataset.theme = dark ? 'dark' : 'light';
	document.querySelectorAll('.theme-btn').forEach(btn => {
		btn.classList.toggle('active', btn.dataset.theme === t);
	});
}

document.querySelectorAll('.theme-btn').forEach(btn => {
	btn.addEventListener('click', () => setTheme(btn.dataset.theme));
});

// Sync active state on load
const saved = localStorage.getItem('runbooks-theme') || 'dark';
document.querySelectorAll('.theme-btn').forEach(btn => {
	btn.classList.toggle('active', btn.dataset.theme === saved);
});

// Code weight switcher (applies --mono-weight via data-code-weight)
function setWeight(w) {
	localStorage.setItem('runbooks-code-weight', w);
	html.dataset.codeWeight = w;
	document.querySelectorAll('.weight-btn').forEach(btn => {
		btn.classList.toggle('active', btn.dataset.weight === w);
	});
}

document.querySelectorAll('.weight-btn').forEach(btn => {
	btn.addEventListener('click', () => setWeight(btn.dataset.weight));
});

const savedWeight = localStorage.getItem('runbooks-code-weight') || 'regular';
document.querySelectorAll('.weight-btn').forEach(btn => {
	btn.classList.toggle('active', btn.dataset.weight === savedWeight);
});

// Reading size (applies --text-scale via data-text-size)
function setSize(s) {
	localStorage.setItem('runbooks-text-size', s);
	html.dataset.textSize = s;
	document.querySelectorAll('.size-btn').forEach(btn => {
		btn.classList.toggle('active', btn.dataset.size === s);
	});
}

document.querySelectorAll('.size-btn').forEach(btn => {
	btn.addEventListener('click', () => setSize(btn.dataset.size));
});

const savedSize = localStorage.getItem('runbooks-text-size') || 'medium';
document.querySelectorAll('.size-btn').forEach(btn => {
	btn.classList.toggle('active', btn.dataset.size === savedSize);
});

// Collapsible steps. The collapsed state is a class set here, before first
// paint (this script is synchronous at the end of the body), so there is no
// flash of expanded steps.
function setCardState(card, open) {
	card.classList.toggle('collapsed', !open);
	card.querySelector('.step-toggle')?.setAttribute('aria-expanded', String(open));
}
function applySteps(pref) {
	[...document.querySelectorAll('[data-runbook] .step-card')].forEach((card, i) =>
		setCardState(card, pref === 'all' ? true : i === 0));
}
applySteps(localStorage.getItem('runbooks-steps') || 'first');

// Steps preference (first open, or all open) — global, from the Appearance control.
const stepsBtns = document.querySelectorAll('.steps-btn');
stepsBtns.forEach(btn => btn.addEventListener('click', () => {
	const pref = btn.dataset.steps;
	localStorage.setItem('runbooks-steps', pref);
	stepsBtns.forEach(b => b.classList.toggle('active', b.dataset.steps === pref));
	applySteps(pref);
}));
stepsBtns.forEach(btn => btn.classList.toggle('active', btn.dataset.steps === (localStorage.getItem('runbooks-steps') || 'first')));

// A runbook page also gets the optional Contents list and the bulk controls.
const runbookRoot = document.querySelector('[data-runbook]');
if (runbookRoot) {
	const toc = runbookRoot.querySelector('.toc');
	const contentsBtn = runbookRoot.querySelector('.contents-btn');

	const setContents = (on) => {
		if (toc) toc.hidden = !on;
		if (contentsBtn) {
			contentsBtn.classList.toggle('active', on);
			contentsBtn.setAttribute('aria-pressed', String(on));
		}
	};
	setContents(localStorage.getItem('runbooks-contents') === 'on');

	// Toggle one step: the chevron button, or a click on the header (except the
	// done checkbox).
	runbookRoot.querySelectorAll('.step-card').forEach(card => {
		const toggle = () => setCardState(card, card.classList.contains('collapsed'));
		card.querySelector('.step-toggle')?.addEventListener('click', toggle);
		card.querySelector('.step-header')?.addEventListener('click', e => {
			if (e.target.closest('.step-check') || e.target.closest('.step-toggle')) return;
			toggle();
		});
	});

	runbookRoot.querySelector('[data-steps-action="expand"]')?.addEventListener('click', () => applySteps('all'));
	runbookRoot.querySelector('[data-steps-action="collapse"]')?.addEventListener('click', () => {
		runbookRoot.querySelectorAll('.step-card').forEach(card => setCardState(card, false));
	});

	contentsBtn?.addEventListener('click', () => {
		const on = contentsBtn.getAttribute('aria-pressed') !== 'true';
		setContents(on);
		localStorage.setItem('runbooks-contents', on ? 'on' : 'off');
	});

	// Zen mode: hide the chrome, one step at a time. Esc exits.
	const zenCards = [...runbookRoot.querySelectorAll('.step-card:not(.rollback-card)')];
	const zenCount = document.querySelector('.zen-count');
	let zenCurrent = 0;

	const setZen = (on, current = zenCurrent) => {
		document.body.classList.toggle('zen', on);
		if (!on) {
			localStorage.setItem('runbooks-zen', 'off');
			applySteps(localStorage.getItem('runbooks-steps') || 'first');
			return;
		}
		zenCurrent = Math.max(0, Math.min(current, zenCards.length - 1));
		zenCards.forEach((card, i) => setCardState(card, i === zenCurrent));
		if (zenCount) zenCount.textContent = `Step ${zenCurrent + 1} of ${zenCards.length}`;
	};

	document.querySelector('[data-zen-action="enter"]')?.addEventListener('click', () => {
		localStorage.setItem('runbooks-zen', 'on');
		setZen(true, 0);
	});
	document.querySelector('[data-zen-action="exit"]')?.addEventListener('click', () => setZen(false));
	document.querySelector('[data-zen-action="prev"]')?.addEventListener('click', () => setZen(true, zenCurrent - 1));
	document.querySelector('[data-zen-action="next"]')?.addEventListener('click', () => setZen(true, zenCurrent + 1));
	document.addEventListener('keydown', e => {
		if (e.key === 'Escape' && document.body.classList.contains('zen')) setZen(false);
	});

	if (localStorage.getItem('runbooks-zen') === 'on') setZen(true, 0);
}

// Destructive runbooks: gate the page behind an acknowledgement, once per
// session. The dialog cannot be dismissed — only accepted.
const ackDialog = document.querySelector('[data-ack-dialog]');
if (ackDialog) {
	const gated = document.querySelector('[data-ack]');
	const badge = document.querySelector('[data-ack-badge]');
	const key = 'runbooks-ack:' + location.pathname;
	const clearGate = () => {
		if (gated) {
			gated.removeAttribute('inert');
			gated.dataset.ack = 'done';
		}
	};
	// Leave a reminder in the header that this destructive runbook was accepted.
	const showBadge = (at) => {
		if (!badge) return;
		badge.hidden = false;
		if (at) badge.title = `You acknowledged this runbook this session at ${new Date(at).toLocaleTimeString()}`;
	};

	const ackedAt = sessionStorage.getItem(key);
	if (ackedAt) {
		clearGate();
		showBadge(ackedAt);
	} else {
		ackDialog.addEventListener('cancel', e => e.preventDefault());
		ackDialog.querySelector('[data-ack-accept]')?.addEventListener('click', () => {
			const at = new Date().toISOString();
			sessionStorage.setItem(key, at);
			clearGate();
			showBadge(at);
			ackDialog.close();
			// Best-effort audit record (only when identity is on).
			const endpoint = ackDialog.dataset.ackEndpoint;
			if (endpoint) {
				fetch(endpoint, {
					method: 'POST',
					headers: { 'Content-Type': 'application/json' },
					body: JSON.stringify({ slug: ackDialog.dataset.ackSlug }),
				}).catch(() => {});
			}
		});
		ackDialog.showModal();
	}
}

// Off-canvas drawers: the rail summons the sidebar at every width, while the
// notes panel is a drawer only below 1200px (above, it is a column). The scrim,
// Esc and ✕ close whichever is open.
const narrowShell = window.matchMedia('(max-width: 1199.98px)');

function setDrawer(name) {
	if (name) document.body.dataset.drawer = name;
	else delete document.body.dataset.drawer;
	document.querySelector('[data-rail-menu]')?.setAttribute('aria-expanded', String(name === 'sidebar'));
	if (narrowShell.matches) setNotesGlyph(name === 'notes');
}

document.querySelector('[data-rail-menu]')?.addEventListener('click', () => {
	setDrawer(document.body.dataset.drawer === 'sidebar' ? null : 'sidebar');
});
document.querySelector('[data-drawer-scrim]')?.addEventListener('click', () => setDrawer(null));
document.querySelectorAll('[data-drawer-close]').forEach(btn => btn.addEventListener('click', () => {
	// The notes panel floats at every width: its close hides the panel on wide
	// layouts and closes the drawer on narrow ones.
	if (btn.closest('.notes-panel') && !narrowShell.matches) {
		setNotesShown(false);
		localStorage.setItem('runbooks-notes', 'off');
		return;
	}
	setDrawer(null);
}));
document.addEventListener('keydown', e => {
	if (e.key === 'Escape' && document.body.dataset.drawer) setDrawer(null);
});

// Appearance popover: the rail cog and the sidebar strip toggle the single
// floating panel (hidden by default). Clicks outside the panel or Esc close it;
// the drawer handlers' outside-click sits above this in the file order, so a
// click that lands on the panel does not dismiss the sidebar underneath.
const appearancePanel = document.querySelector('[data-appearance-panel]');
if (appearancePanel) {
	// The styleguide renders a static specimen of the panel; its trigger must not
	// open the real one (which would stack a second panel over the example).
	const appearanceToggles = [...document.querySelectorAll('[data-appearance-toggle]')].filter(
		btn => !btn.closest('[data-appearance-specimen]'),
	);
	const setAppearance = (open) => {
		appearancePanel.hidden = !open;
		appearanceToggles.forEach(btn => btn.setAttribute('aria-expanded', String(open)));
	};
	appearanceToggles.forEach(btn => btn.addEventListener('click', () =>
		setAppearance(appearancePanel.hidden)));
	document.addEventListener('click', e => {
		if (!appearancePanel.hidden && !e.target.closest('[data-appearance-panel], [data-appearance-toggle]')) {
			setAppearance(false);
		}
	});
	document.addEventListener('keydown', e => {
		if (e.key === 'Escape' && !appearancePanel.hidden) setAppearance(false);
	});
}

const notesToggle = document.querySelector('[data-notes-toggle]');
if (notesToggle) {
	const applyNotes = setNotesShown;
	// Wide: the persisted preference governs the notes column. Narrow: notes is a
	// closed drawer, opened on demand.
	if (!narrowShell.matches) applyNotes(localStorage.getItem('runbooks-notes') !== 'off');
	notesToggle.addEventListener('click', () => {
		if (narrowShell.matches) {
			setDrawer(document.body.dataset.drawer === 'notes' ? null : 'notes');
			setNotesGlyph(document.body.dataset.drawer === 'notes');
			return;
		}
		const show = document.body.classList.contains('notes-hidden');
		localStorage.setItem('runbooks-notes', show ? 'on' : 'off');
		applyNotes(show);
	});
	narrowShell.addEventListener('change', () => {
		setDrawer(null);
		if (narrowShell.matches) document.body.classList.remove('notes-hidden');
		else applyNotes(localStorage.getItem('runbooks-notes') !== 'off');
	});
}

// Index page — server-side body search over the runbook catalogue. The query is
// debounced to the JSON endpoint; the catalogue is the no-query / fallback state.
const indexSearch = document.querySelector('.index-search');
if (indexSearch) {
	const results = document.querySelector('[data-search-results]');
	const catalogue = document.querySelector('[data-search-catalogue]');
	const symptomLinks = document.querySelector('.search-chips');
	const searchKey = document.querySelector('[data-search-key]');
	const searchClear = document.querySelector('[data-search-clear]');
	let timer = null;
	let controller = null;

	const setChrome = q => {
		if (symptomLinks) symptomLinks.hidden = q.length > 0;
		if (searchKey) searchKey.hidden = q.length > 0;
		if (searchClear) searchClear.hidden = q.length === 0;
	};

	const showCatalogue = () => {
		if (results) {
			results.hidden = true;
			results.replaceChildren();
		}
		if (catalogue) catalogue.hidden = false;
	};

	// A message in the results slot; with keepCatalogue the fallback list stays
	// visible underneath, so a failed fetch never strands the reader on a blank page.
	const showMessage = (msg, keepCatalogue) => {
		if (catalogue) catalogue.hidden = !keepCatalogue;
		if (!results) return;
		const p = document.createElement('p');
		p.className = 'index-results-empty';
		p.textContent = msg;
		results.replaceChildren(p);
		results.hidden = false;
	};

	const renderResults = data => {
		if (!results) return;
		results.replaceChildren();
		if (!data.results.length) {
			showMessage('No runbooks match that. Try another word, or clear the search.', false);
			return;
		}
		const list = document.createElement('ul');
		list.className = 'search-result-list';
		for (const r of data.results) {
			const li = document.createElement('li');
			li.className = 'search-result';
			const link = document.createElement('a');
			link.className = 'search-result-link';
			link.href = '/' + r.slug + (r.stepAnchor ? '#' + r.stepAnchor : '');

			const title = document.createElement('span');
			title.className = 'search-result-title';
			title.textContent = r.title;
			link.append(title);

			const where = [[r.system, r.category].filter(Boolean).join(' › '), r.step]
				.filter(Boolean)
				.join(' · ');
			if (where) {
				const meta = document.createElement('span');
				meta.className = 'search-result-meta';
				meta.textContent = where;
				link.append(meta);
			}

			const snippet = document.createElement('span');
			snippet.className = 'search-result-snippet';
			// The server escapes and <mark>-wraps the snippet; this is the only HTML
			// the client ever injects from the response.
			setHTML(snippet, r.snippet);
			link.append(snippet);

			li.append(link);
			list.append(li);
		}
		results.append(list);
		results.hidden = false;
		if (catalogue) catalogue.hidden = true;
	};

	const run = async q => {
		if (controller) controller.abort();
		controller = new AbortController();
		try {
			const res = await fetch(`/api/runbooks/v1/search?q=${encodeURIComponent(q)}`, {
				headers: { Accept: 'application/json' },
				signal: controller.signal,
			});
			if (!res.ok) throw new Error(`search ${res.status}`);
			renderResults(await res.json());
		} catch (err) {
			if (err.name === 'AbortError') return;
			showMessage('Search is unavailable right now — showing all runbooks.', true);
		}
	};

	const onInput = () => {
		const q = indexSearch.value.trim();
		setChrome(q);
		clearTimeout(timer);
		if (!q) {
			if (controller) controller.abort();
			showCatalogue();
			return;
		}
		timer = setTimeout(() => run(q), 150);
	};

	indexSearch.addEventListener('input', onInput);
	if (searchClear) {
		searchClear.addEventListener('click', () => {
			indexSearch.value = '';
			onInput();
			indexSearch.focus();
		});
	}
}

// Sidebar — filter and disclosure over the runbook tree
const navFilter = document.querySelector('.nav-filter-input');
const navTree = document.querySelector('[data-nav-tree]');
if (navFilter && navTree) {
	const norm = s => (s || '').toLowerCase();
	// Same rule as the index filter, over the same runbook search text: phrase
	// match first, then "every word appears".
	const matches = (hay, q) => !q || hay.includes(q) || q.split(/\s+/).every(t => hay.includes(t));
	const links = [...navTree.querySelectorAll('.nav-links a')];
	const sections = [...navTree.querySelectorAll('[data-nav-sys], [data-nav-cat]')];
	const navResults = document.querySelector('[data-nav-results]');
	const navClear = document.querySelector('[data-nav-filter-clear]');
	const navKey = document.querySelector('[data-nav-filter-key]');
	// The branch the server opened for the active runbook, restored on clear.
	const openAtLoad = new Set(sections.filter(s => s.classList.contains('open')));

	const applyNavFilter = () => {
		const q = norm(navFilter.value.trim());
		let visible = 0;
		links.forEach(a => {
			const hit = matches(norm(a.dataset.search), q);
			a.hidden = !hit;
			if (hit) visible++;
		});
		sections.forEach(section => {
			const hits = [...section.querySelectorAll('.nav-links a')].filter(a => !a.hidden).length;
			const open = q ? hits > 0 : openAtLoad.has(section);
			section.hidden = q.length > 0 && hits === 0;
			section.classList.toggle('open', open);
			const toggle = section.querySelector('[data-nav-toggle]');
			if (toggle) toggle.setAttribute('aria-expanded', String(open));
			const count = section.querySelector('[data-nav-count]');
			if (count) count.textContent = q ? String(hits) : count.dataset.total;
		});
		if (navResults) {
			navResults.hidden = !q;
			navResults.textContent = visible === 1 ? '1 runbook matches' : `${visible} runbooks match`;
		}
		if (navClear) navClear.hidden = !q;
		if (navKey) navKey.hidden = !!q;
	};

	const clearNavFilter = () => {
		navFilter.value = '';
		applyNavFilter();
	};

	navFilter.addEventListener('input', applyNavFilter);
	navFilter.addEventListener('keydown', e => {
		if (e.key === 'Escape') {
			clearNavFilter();
			navFilter.blur();
		}
	});
	if (navClear) {
		navClear.addEventListener('click', () => {
			clearNavFilter();
			navFilter.focus();
		});
	}
	navTree.querySelectorAll('[data-nav-toggle]').forEach(btn => {
		btn.addEventListener('click', () => {
			const section = btn.closest('[data-nav-sys], [data-nav-cat]');
			const open = section.classList.toggle('open');
			btn.setAttribute('aria-expanded', String(open));
		});
	});
	// `/` focuses a filter, unless a field already has focus. On the index page the
	// search hub is the primary field, so it wins — unless the sidebar drawer is
	// open, where the nav filter keeps it.
	document.addEventListener('keydown', e => {
		if (e.key !== '/' || e.metaKey || e.ctrlKey || e.altKey) return;
		const el = document.activeElement;
		if (el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.tagName === 'SELECT' || el.isContentEditable)) return;
		e.preventDefault();
		if (indexSearch && document.body.dataset.drawer !== 'sidebar') indexSearch.focus();
		else navFilter.focus();
	});
}

// Git Sync button — only active when rendered by server (GitSyncEnabled = true)
const notesSync = document.querySelector('.notes-sync');
const syncToastEl = document.querySelector('.notes-sync-toast');
let syncToastTimer = null;

function showSyncToast(type, msg) {
	if (!syncToastEl) return;
	clearTimeout(syncToastTimer);
	syncToastEl.className = `notes-sync-toast notes-sync-toast--${type}`;
	syncToastEl.querySelector('.notes-sync-toast-msg').textContent = msg;
	syncToastEl.hidden = false;
	syncToastTimer = setTimeout(hideSyncToast, 5000);
}

function hideSyncToast() {
	clearTimeout(syncToastTimer);
	if (syncToastEl) syncToastEl.hidden = true;
}

if (syncToastEl) {
	syncToastEl.querySelector('.notes-sync-toast-close').addEventListener('click', hideSyncToast);
}

// Shared instance bearer for the sync endpoint. Held in sessionStorage only, so
// it is gone when the browser closes; never localStorage.
async function syncBearer() {
	if (!pageConfig.gitSyncRequiresToken) return null;
	let token = sessionStorage.getItem('runbooks-gitsync-token');
	if (!token) {
		const { confirmed, value } = await showDialog({
			title: 'Git sync token',
			message: 'Enter the bearer token for this runbooks instance. It is kept for this tab only.',
			confirmLabel: 'Continue',
			input: { type: 'password', placeholder: 'Bearer token' },
		});
		token = confirmed ? value : null;
		if (token) sessionStorage.setItem('runbooks-gitsync-token', token);
	}
	return token;
}

if (notesSync && notesArea) {
	function updateSyncDisabled() {
		notesSync.disabled = !notesArea.value.trim();
	}
	updateSyncDisabled();
	notesArea.addEventListener('input', updateSyncDisabled);

	notesSync.addEventListener('click', async () => {
		if (notesSync.disabled) return;
		const bearer = await syncBearer();
		if (pageConfig.gitSyncRequiresToken && !bearer) {
			showSyncToast('error', 'A bearer token is required to sync');
			return;
		}
		notesSync.disabled = true;
		notesSync.classList.add('loading');
		try {
			const title = document.title.replace(/\s*—.*$/, '').trim();
			const slug = location.pathname.slice(1).replace(/\//g, '-');
			const imgs = JSON.parse(localStorage.getItem(notesImgKey) || '{}');
			const images = Object.entries(imgs).map(([name, src]) => {
				const raw = (src.match(/^data:image\/(\w+)/) || ['', 'png'])[1];
				return { name, mime_type: `image/${raw}`, data_base64: src.split(',')[1] || '' };
			});
			const sourceEl = document.getElementById('runbook-source');
			const runbookSource = sourceEl ? JSON.parse(sourceEl.textContent) : '';
			const headers = { 'Content-Type': 'application/json' };
			if (bearer) headers.Authorization = `Bearer ${bearer}`;
			const res = await fetch('/api/git-sync/v1', {
				method: 'POST',
				headers,
				body: JSON.stringify({
					runbook_slug: slug,
					runbook_title: title,
					notes: notesArea.value,
					runbook_source: runbookSource,
					images,
				}),
			});
			const data = await res.json().catch(() => ({}));
			if (res.ok) {
				const msg = data.commit_sha ? `Synced (${data.commit_sha.slice(0, 7)})` : 'Already up to date';
				showSyncToast('success', msg);
			} else if (res.status === 401) {
				sessionStorage.removeItem('runbooks-gitsync-token');
				showSyncToast('error', 'Unauthorized — token rejected');
			} else {
				showSyncToast('error', data.error || 'Sync failed');
			}
		} catch (_) {
			showSyncToast('error', 'Network error — sync failed');
		} finally {
			notesSync.classList.remove('loading');
			updateSyncDisabled();
		}
	});
}

async function authPost(url, body) {
	const res = await fetch(url, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify(body || {}),
	});
	const data = await res.json().catch(() => ({}));
	if (!res.ok) throw new Error(data.error || `request failed (${res.status})`);
	return data;
}

function authStatus(message) {
	const el = document.querySelector('[data-auth-status]');
	if (!el) return;
	el.textContent = message;
	el.hidden = !message;
	if (message) hideLiveAlert();
}

// passkeyErrorMessage turns the terse DOMException the WebAuthn API throws into
// something a person can act on. The raw text still reaches the alert's detail
// line, so nothing is lost for debugging.
function passkeyErrorMessage(err) {
	const fallbacks = {
		NotAllowedError:
			'The passkey request was cancelled or timed out. Try again, or use the device where the passkey already lives.',
		InvalidStateError: 'That passkey is already registered on this device — try signing in instead.',
		AbortError: 'The passkey request was interrupted. Please try again.',
		SecurityError: 'This browser blocked the passkey request. Check that the site is served over HTTPS.',
	};
	return fallbacks[err && err.name] || (err && err.message) || 'Something went wrong.';
}

// showLiveAlert fills the page's panel alert. The weight is fixed in markup;
// only the text and the optional raw detail vary here.
function showLiveAlert(title, err) {
	const el = document.querySelector('[data-live-alert]');
	if (!el) return;
	const message = passkeyErrorMessage(err);
	const detail = message === (err && err.message) ? '' : err && err.message;
	el.querySelector('.alert-title').textContent = title;
	el.querySelector('.alert-message').textContent = message;
	const detailEl = el.querySelector('.alert-detail');
	detailEl.textContent = detail || '';
	detailEl.hidden = !detail;
	el.hidden = false;
	authStatus('');
	const adminStatus = document.querySelector('[data-admin-status]');
	if (adminStatus) adminStatus.hidden = true;
}

function hideLiveAlert() {
	const el = document.querySelector('[data-live-alert]');
	if (el) el.hidden = true;
}

// createPasskey runs the WebAuthn registration ceremony and returns the
// credential JSON the server expects.
async function createPasskey(options) {
	const pk = options.publicKey;
	pk.challenge = decodeBase64URL(pk.challenge);
	pk.user.id = decodeBase64URL(pk.user.id);
	(pk.excludeCredentials || []).forEach(c => {
		c.id = decodeBase64URL(c.id);
	});
	const cred = await navigator.credentials.create({ publicKey: pk });
	return {
		id: cred.id,
		rawId: encodeBase64URL(cred.rawId),
		type: cred.type,
		response: {
			clientDataJSON: encodeBase64URL(cred.response.clientDataJSON),
			attestationObject: encodeBase64URL(cred.response.attestationObject),
			transports: cred.response.getTransports ? cred.response.getTransports() : undefined,
		},
	};
}

// getPasskey runs the WebAuthn assertion ceremony and returns the credential
// JSON the server expects.
async function getPasskey(options) {
	const pk = options.publicKey;
	pk.challenge = decodeBase64URL(pk.challenge);
	(pk.allowCredentials || []).forEach(c => {
		c.id = decodeBase64URL(c.id);
	});
	const cred = await navigator.credentials.get({ publicKey: pk });
	return {
		id: cred.id,
		rawId: encodeBase64URL(cred.rawId),
		type: cred.type,
		response: {
			clientDataJSON: encodeBase64URL(cred.response.clientDataJSON),
			authenticatorData: encodeBase64URL(cred.response.authenticatorData),
			signature: encodeBase64URL(cred.response.signature),
			userHandle: cred.response.userHandle ? encodeBase64URL(cred.response.userHandle) : undefined,
		},
	};
}

// Sign out (sidebar footer). The endpoint only exists when identity is on.
const logoutButton = document.querySelector('[data-auth="logout"]');
if (logoutButton) {
	logoutButton.addEventListener('click', async () => {
		logoutButton.disabled = true;
		try {
			await fetch('/api/auth/v1/logout', { method: 'POST' });
		} finally {
			// A fresh sign-in re-acknowledges destructive runbooks: the acknowledgement
			// is per session, and signing out ends the session.
			Object.keys(sessionStorage)
				.filter(k => k.startsWith('runbooks-ack:'))
				.forEach(k => sessionStorage.removeItem(k));
			window.location.assign('/login');
		}
	});
}

const loginButton = document.querySelector('[data-auth="login"]');
if (loginButton) {
	loginButton.addEventListener('click', async () => {
		loginButton.disabled = true;
		hideLiveAlert();
		authStatus('Waiting for your passkey…');
		try {
			const begin = await authPost('/api/auth/v1/login/begin');
			const credential = await getPasskey(begin.options);
			await authPost('/api/auth/v1/login/finish', { challenge: begin.challenge, credential });
			window.location.assign('/');
		} catch (err) {
			showLiveAlert('Sign in failed', err);
			loginButton.disabled = false;
		}
	});
}

const setupForm = document.querySelector('[data-auth="setup"]');
if (setupForm) {
	setupForm.addEventListener('submit', async event => {
		event.preventDefault();
		const fields = Object.fromEntries(new FormData(setupForm).entries());
		const submit = setupForm.querySelector('button[type="submit"]');
		if (submit) submit.disabled = true;
		hideLiveAlert();
		authStatus('Creating your passkey…');
		try {
			const begin = await authPost('/api/auth/v1/setup/begin', {
				token: fields.token,
				display_name: fields.display_name,
				email: fields.email,
			});
			const credential = await createPasskey(begin.options);
			await authPost('/api/auth/v1/setup/finish', {
				token: fields.token,
				user_id: begin.user_id,
				challenge: begin.challenge,
				credential,
			});
			window.location.assign('/');
		} catch (err) {
			showLiveAlert('Setup failed', err);
			if (submit) submit.disabled = false;
		}
	});
}

const inviteForm = document.querySelector('[data-auth="invite"]');
if (inviteForm) {
	inviteForm.addEventListener('submit', async event => {
		event.preventDefault();
		const token = inviteForm.dataset.inviteToken;
		const fields = Object.fromEntries(new FormData(inviteForm).entries());
		const submit = inviteForm.querySelector('button[type="submit"]');
		if (submit) submit.disabled = true;
		hideLiveAlert();
		authStatus('Creating your passkey…');
		try {
			const begin = await authPost('/api/auth/v1/invite/begin', {
				token,
				display_name: fields.display_name,
				email: fields.email,
			});
			const credential = await createPasskey(begin.options);
			await authPost('/api/auth/v1/invite/finish', { token, challenge: begin.challenge, credential });
			window.location.assign('/');
		} catch (err) {
			showLiveAlert('Could not join', err);
			if (submit) submit.disabled = false;
		}
	});
}

const recoveryForm = document.querySelector('[data-auth="recovery"]');
if (recoveryForm) {
	recoveryForm.addEventListener('submit', async event => {
		event.preventDefault();
		const fields = Object.fromEntries(new FormData(recoveryForm).entries());
		const submit = recoveryForm.querySelector('button[type="submit"]');
		if (submit) submit.disabled = true;
		hideLiveAlert();
		authStatus('Creating your passkey…');
		try {
			const begin = await authPost('/api/auth/v1/recovery/begin', { token: fields.token });
			const credential = await createPasskey(begin.options);
			await authPost('/api/auth/v1/recovery/finish', { token: fields.token, challenge: begin.challenge, credential });
			window.location.assign('/');
		} catch (err) {
			showLiveAlert('Recovery failed', err);
			if (submit) submit.disabled = false;
		}
	});
}

const adminStatus = document.querySelector('[data-admin-status]');
if (adminStatus) {
	const adminResult = document.querySelector('[data-admin-result]');
	const adminUrl = adminResult.querySelector('[data-admin-url]');

	const showAdminStatus = message => {
		hideLiveAlert();
		adminStatus.textContent = message;
		adminStatus.hidden = !message;
	};
	const showAdminUrl = url => {
		adminUrl.value = url;
		adminResult.hidden = false;
		adminUrl.focus();
		adminUrl.select();
	};
	const createAdminInvite = async body => {
		try {
			const data = await authPost('/api/auth/v1/invites', body);
			showAdminStatus('Invite link created — it can only be used once.');
			showAdminUrl(data.url);
		} catch (err) {
			showLiveAlert('Could not create the invite', err);
		}
	};

	const adminInviteForm = document.querySelector('[data-admin="invite"]');
	if (adminInviteForm) {
		adminInviteForm.addEventListener('submit', async event => {
			event.preventDefault();
			const fields = Object.fromEntries(new FormData(adminInviteForm).entries());
			await createAdminInvite({ role: fields.role, ttl: fields.ttl });
		});
	}

	document.querySelectorAll('[data-admin="reenrol"]').forEach(button => {
		button.addEventListener('click', () => createAdminInvite({ user_id: button.dataset.userId }));
	});

	document.querySelectorAll('[data-admin="revoke"]').forEach(button => {
		button.addEventListener('click', async () => {
			button.disabled = true;
			try {
				await authPost('/api/auth/v1/sessions/revoke', { user_id: button.dataset.userId });
				showAdminStatus('Sessions revoked.');
			} catch (err) {
				showLiveAlert('Could not revoke sessions', err);
			} finally {
				button.disabled = false;
			}
		});
	});

	const setUserEnabled = (selector, path, verb) => {
		document.querySelectorAll(selector).forEach(button => {
			button.addEventListener('click', async () => {
				button.disabled = true;
				try {
					await authPost(path, { user_id: button.dataset.userId });
					window.location.reload();
				} catch (err) {
					showLiveAlert(`Could not ${verb} the user`, err);
					button.disabled = false;
				}
			});
		});
	};
	setUserEnabled('[data-admin="disable"]', '/api/auth/v1/users/disable', 'disable');
	setUserEnabled('[data-admin="enable"]', '/api/auth/v1/users/enable', 'enable');

	const adminCopy = document.querySelector('[data-admin-copy]');
	if (adminCopy) {
		adminCopy.addEventListener('click', async () => {
			try {
				await navigator.clipboard.writeText(adminUrl.value);
				showAdminStatus('Copied.');
			} catch (_) {
				adminUrl.select();
				showAdminStatus('Press Ctrl/Cmd+C to copy.');
			}
		});
	}
}

// Account page: profile, passkeys and signed-in devices.
const accountForm = document.querySelector('[data-account="profile"]');
if (accountForm) {
	accountForm.addEventListener('submit', async event => {
		event.preventDefault();
		const fields = Object.fromEntries(new FormData(accountForm).entries());
		try {
			await authPost('/api/auth/v1/profile', { display_name: fields.display_name, email: fields.email });
			authStatus('Profile saved.');
		} catch (err) {
			showLiveAlert('Could not save your profile', err);
		}
	});

	const addPasskey = document.querySelector('[data-account="add-passkey"]');
	if (addPasskey) {
		addPasskey.addEventListener('click', async () => {
			addPasskey.disabled = true;
			hideLiveAlert();
			authStatus('Creating your passkey…');
			try {
				const begin = await authPost('/api/auth/v1/passkeys/begin');
				const credential = await createPasskey(begin.options);
				await authPost('/api/auth/v1/passkeys/finish', { challenge: begin.challenge, credential });
				window.location.reload();
			} catch (err) {
				showLiveAlert('Could not add the passkey', err);
				addPasskey.disabled = false;
			}
		});
	}

	document.querySelectorAll('[data-account="save-passkey"]').forEach(button => {
		button.addEventListener('click', async () => {
			const row = button.closest('tr');
			const input = row.querySelector('[data-passkey-label]');
			button.disabled = true;
			try {
				await authPost('/api/auth/v1/passkeys/rename', {
					credential_id: button.dataset.credentialId,
					label: input.value,
				});
				authStatus('Passkey name saved.');
			} catch (err) {
				showLiveAlert('Could not rename the passkey', err);
			} finally {
				button.disabled = false;
			}
		});
	});

	document.querySelectorAll('[data-account="remove-passkey"]').forEach(button => {
		button.addEventListener('click', async () => {
			button.disabled = true;
			try {
				await authPost('/api/auth/v1/passkeys/remove', { credential_id: button.dataset.credentialId });
				button.closest('tr').remove();
				authStatus('Passkey removed.');
			} catch (err) {
				showLiveAlert('Could not remove the passkey', err);
				button.disabled = false;
			}
		});
	});

	document.querySelectorAll('[data-account="revoke-session"]').forEach(button => {
		button.addEventListener('click', async () => {
			button.disabled = true;
			try {
				await authPost('/api/auth/v1/sessions/revoke', { session_id: button.dataset.sessionId });
				if (button.dataset.current === 'true') {
					window.location.assign('/login');
					return;
				}
				button.closest('tr').remove();
				authStatus('Device signed out.');
			} catch (err) {
				showLiveAlert('Could not sign out that device', err);
				button.disabled = false;
			}
		});
	});

	const signOutAll = document.querySelector('[data-account="sign-out-all"]');
	if (signOutAll) {
		signOutAll.addEventListener('click', async () => {
			signOutAll.disabled = true;
			try {
				await authPost('/api/auth/v1/sessions/revoke', { user_id: signOutAll.dataset.userId });
				window.location.assign('/login');
			} catch (err) {
				showLiveAlert('Could not sign out', err);
				signOutAll.disabled = false;
			}
		});
	}

	const createAPIKey = document.querySelector('[data-account="create-apikey"]');
	if (createAPIKey) {
		createAPIKey.addEventListener('submit', async event => {
			event.preventDefault();
			const button = createAPIKey.querySelector('button[type="submit"]');
			const label = createAPIKey.querySelector('[name="label"]').value.trim();
			button.disabled = true;
			try {
				const result = await authPost('/api/auth/v1/apikeys', { label });
				document.querySelector('[data-apikey-value]').textContent = result.raw;
				document.querySelector('[data-apikey-reveal]').hidden = false;
				createAPIKey.reset();
				authStatus('Key created. Copy it now — it is shown once.');
			} catch (err) {
				showLiveAlert('Could not create the key', err);
			} finally {
				button.disabled = false;
			}
		});
	}

	const copyAPIKey = document.querySelector('[data-account="copy-apikey"]');
	if (copyAPIKey) {
		copyAPIKey.addEventListener('click', async () => {
			const value = document.querySelector('[data-apikey-value]').textContent;
			try {
				await navigator.clipboard.writeText(value);
				authStatus('Key copied.');
			} catch {
				showLiveAlert('Could not copy the key', new Error('clipboard unavailable'));
			}
		});
	}

	document.querySelectorAll('[data-account="revoke-apikey"]').forEach(button => {
		button.addEventListener('click', async () => {
			button.disabled = true;
			try {
				await authPost('/api/auth/v1/apikeys/revoke', { key_id: button.dataset.keyId });
				window.location.reload();
			} catch (err) {
				showLiveAlert('Could not revoke the key', err);
				button.disabled = false;
			}
		});
	});
}
