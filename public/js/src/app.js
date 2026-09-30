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
		if (code) code.innerHTML = renderBlock(block.dataset.template, block.dataset.lang || '');
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
			recordTimeline(blockTimelineEntry(group, group.classList.toggle('done')));
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

	// Position popup to the right of the sidebar, vertically near the button.
	// Popup uses visibility:hidden so offsetHeight is always valid.
	const sidebarWidth = document.querySelector('.sidebar').offsetWidth;
	const r = btn.getBoundingClientRect();
	const popupH = hintPopup.offsetHeight;
	const top = Math.max(8, Math.min(r.top, window.innerHeight - popupH - 8));

	hintPopup.style.left = (sidebarWidth + 8) + 'px';
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
const notesKey = 'll-notes' + location.pathname;
const notesImgKey = 'll-notes-imgs' + location.pathname;
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

if (notesArea) {
	notesArea.value = localStorage.getItem(notesKey) || '';
	notesArea.addEventListener('input', () => localStorage.setItem(notesKey, notesArea.value));

	notesArea.addEventListener('paste', e => {
		const imgItem = Array.from(e.clipboardData?.items || []).find(i => i.type.startsWith('image/'));
		if (!imgItem) return;
		e.preventDefault();
		const reader = new FileReader();
		reader.onload = ev => {
			const imgs = JSON.parse(localStorage.getItem(notesImgKey) || '{}');
			const n = Object.keys(imgs).filter(k => k.startsWith('img-')).length + 1;
			const id = `img-${n}`;
			imgs[id] = ev.target.result;
			localStorage.setItem(notesImgKey, JSON.stringify(imgs));
			const md = `\n![screenshot][${id}]\n`;
			const start = notesArea.selectionStart;
			notesArea.value = notesArea.value.slice(0, start) + md + notesArea.value.slice(notesArea.selectionEnd);
			notesArea.selectionStart = notesArea.selectionEnd = start + md.length;
			localStorage.setItem(notesKey, notesArea.value);
		};
		reader.readAsDataURL(imgItem.getAsFile());
	});
}

// Completion timeline — each tick appends a timestamped line to the notes body.
// Entries are plain markdown, so Preview, Print and Export pick them up for free.
const recordKey = 'll-record-timeline';
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
		notesPreview.innerHTML = window.marked.parse(expandNoteImgs(next));
	}
}

function setNotesMode(mode) {
	notesTabs.forEach(b => b.classList.toggle('active', b.dataset.mode === mode));
	if (mode === 'preview') {
		notesPreview.innerHTML = window.marked.parse(expandNoteImgs(notesArea.value || ''));
		notesPreview.hidden = false;
		notesArea.hidden = true;
	} else {
		notesArea.hidden = false;
		notesPreview.hidden = true;
		notesArea.focus();
	}
}

notesTabs.forEach(btn => btn.addEventListener('click', () => setNotesMode(btn.dataset.mode)));

if (notesClear) {
	notesClear.addEventListener('click', () => {
		const ok = window.confirm(
			'Clear the notes, pasted screenshots, the timeline, and all completed steps? This cannot be undone.'
		);
		if (!ok) return;
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

// Notes panel drag-to-resize
const notesHandle = document.querySelector('.notes-resize');
if (notesHandle) {
	notesHandle.addEventListener('mousedown', e => {
		const startX = e.clientX;
		const startW = document.querySelector('.notes-panel').offsetWidth;
		document.body.classList.add('resizing-notes');

		const onMove = e => {
			const w = Math.max(180, Math.min(640, startW + (startX - e.clientX)));
			document.body.style.gridTemplateColumns =
				`var(--size-app-sidebar) 1fr ${w}px`;
		};
		const onUp = () => {
			document.body.classList.remove('resizing-notes');
			document.removeEventListener('mousemove', onMove);
			document.removeEventListener('mouseup', onUp);
		};
		document.addEventListener('mousemove', onMove);
		document.addEventListener('mouseup', onUp);
		e.preventDefault();
	});
}

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

// Index page — live filter over the runbook catalogue
const indexSearch = document.querySelector('.index-search');
if (indexSearch) {
	const norm = s => (s || '').toLowerCase();
	const cards = [...document.querySelectorAll('.runbook-card')];
	const categories = [...document.querySelectorAll('.index-category')];
	const groups = [...document.querySelectorAll('.index-group')];
	const commonSection = document.querySelector('.common-issues');
	const emptyState = document.querySelector('.index-empty');

	// Phrase match first; fall back to "every word appears" so word order and
	// plural/possessive differences still find the runbook.
	const matches = (hay, q) => !q || hay.includes(q) || q.split(/\s+/).every(t => hay.includes(t));

	const applyFilter = () => {
		const q = norm(indexSearch.value.trim());
		let visible = 0;
		cards.forEach(card => {
			const hit = matches(norm(card.dataset.search), q);
			card.hidden = !hit;
			if (hit) visible++;
		});
		categories.forEach(cat => {
			cat.hidden = [...cat.querySelectorAll('.runbook-card')].every(c => c.hidden);
		});
		groups.forEach(group => {
			group.hidden = [...group.querySelectorAll('.index-category')].every(c => c.hidden);
		});
		if (commonSection) commonSection.hidden = q.length > 0;
		if (emptyState) emptyState.hidden = visible > 0 || q.length === 0;
	};

	indexSearch.addEventListener('input', applyFilter);
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
function syncBearer() {
	if (!pageConfig.gitSyncRequiresToken) return null;
	let token = sessionStorage.getItem('ll-gitsync-token');
	if (!token) {
		token = window.prompt('Bearer token for this runbooks instance:');
		if (token) sessionStorage.setItem('ll-gitsync-token', token);
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
		const bearer = syncBearer();
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
				sessionStorage.removeItem('ll-gitsync-token');
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

function authStatus(message, isError) {
	const el = document.querySelector('[data-auth-status]');
	if (!el) return;
	el.textContent = message;
	el.classList.toggle('auth-status--error', Boolean(isError));
	el.hidden = !message;
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

const loginButton = document.querySelector('[data-auth="login"]');
if (loginButton) {
	loginButton.addEventListener('click', async () => {
		loginButton.disabled = true;
		authStatus('Waiting for your passkey…');
		try {
			const begin = await authPost('/api/auth/v1/login/begin');
			const credential = await getPasskey(begin.options);
			await authPost('/api/auth/v1/login/finish', { challenge: begin.challenge, credential });
			window.location.assign('/');
		} catch (err) {
			authStatus(`Sign in failed: ${err.message}`, true);
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
			authStatus(`Setup failed: ${err.message}`, true);
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
			authStatus(`Could not join: ${err.message}`, true);
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
		authStatus('Creating your passkey…');
		try {
			const begin = await authPost('/api/auth/v1/recovery/begin', { token: fields.token });
			const credential = await createPasskey(begin.options);
			await authPost('/api/auth/v1/recovery/finish', { token: fields.token, challenge: begin.challenge, credential });
			window.location.assign('/');
		} catch (err) {
			authStatus(`Recovery failed: ${err.message}`, true);
			if (submit) submit.disabled = false;
		}
	});
}

const adminStatus = document.querySelector('[data-admin-status]');
if (adminStatus) {
	const adminResult = document.querySelector('[data-admin-result]');
	const adminUrl = adminResult.querySelector('[data-admin-url]');

	const showAdminStatus = (message, isError) => {
		adminStatus.textContent = message;
		adminStatus.classList.toggle('auth-status--error', Boolean(isError));
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
			showAdminStatus(err.message, true);
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
				showAdminStatus(err.message, true);
			} finally {
				button.disabled = false;
			}
		});
	});

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
