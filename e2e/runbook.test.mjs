// Runbook-page E2E: the reader's interactions on a real runbook page, against the
// kitchen-sink gallery — the step roll-up, the notes Clear dialog, and both
// themes. Also produces the runbook screenshots.
import { test } from "node:test";
import assert from "node:assert/strict";
import { uiTimeout, startApp, startBrowser, bootAdmin, snap, reportFailure, holdIfAsked } from "./harness.mjs";

test("the runbook page: step roll-up, the clear dialog, and light mode", async (t) => {
	const app = await startApp();
	t.after(() => app.stop());
	const b = await startBrowser();
	t.after(() => b.stop());
	try {
		await bootAdmin(b.page, app.base);
		await b.page.goto(app.base + "/gallery");
		await b.page.fill('.var-input[data-var="HOST"]', "db-2.prod.internal");
		await snap(b.page, "runbook");

		// The roll-up below needs every step open; the default is first-open with
		// the rest collapsed.
		await b.page.evaluate(() => localStorage.setItem("runbooks-steps", "all"));
		await b.page.reload();
		await b.page.fill('.var-input[data-var="HOST"]', "db-2.prod.internal");

		// Ticking every block in a step ticks the step itself.
		const stepWithBlocks = b.page
			.locator(".step-card:not(.rollback-card)")
			.filter({ has: b.page.locator(".code-group") })
			.first();
		const blockChecks = stepWithBlocks.locator(".block-check");
		const blockCount = await blockChecks.count();
		for (let i = 0; i < blockCount; i++) await blockChecks.nth(i).click();
		assert.equal(
			await stepWithBlocks.evaluate(el => el.classList.contains("done")),
			true,
			"step ticks when all its blocks are ticked",
		);

		// The destructive Clear confirmation is a real dialog, not window.confirm(),
		// and Clear lives behind the notes overflow menu.
		await b.page.locator("[data-notes-menu]").click();
		await b.page.locator(".notes-clear").click();
		await b.page.locator("dialog.dialog").waitFor({ timeout: uiTimeout });
		await snap(b.page, "dialog");
		await b.page.locator("dialog.dialog .btn-ghost").click();

		// The notes panel hides and is recalled from the runbook header.
		const notesToggle = b.page.locator("[data-notes-toggle]");
		await notesToggle.click();
		await b.page.locator(".notes-panel").waitFor({ state: "hidden", timeout: uiTimeout });
		assert.equal(await b.page.locator(".notes-panel").isHidden(), true, "notes panel hides");
		await notesToggle.click();
		await b.page.locator(".notes-panel").waitFor({ state: "visible", timeout: uiTimeout });
		assert.equal(await b.page.locator(".notes-panel").isVisible(), true, "notes panel is recalled");

		// Light theme: code surfaces stay dark, the rest inverts.
		await b.page.evaluate(() => localStorage.setItem("runbooks-theme", "light"));
		await b.page.reload();
		await b.page.locator(".code-group").first().scrollIntoViewIfNeeded();
		await snap(b.page, "runbook-light-code");
		await b.page.locator(".rollback-card").first().scrollIntoViewIfNeeded();
		await snap(b.page, "runbook-light-rollback");
		await holdIfAsked();
	} catch (err) {
		await reportFailure(b.page, app.logs());
		throw err;
	}
});

test("the index: body search returns snippets and restores the catalogue", async (t) => {
	const app = await startApp();
	t.after(() => app.stop());
	const b = await startBrowser();
	t.after(() => b.stop());
	try {
		await bootAdmin(b.page, app.base);
		await b.page.goto(app.base + "/");
		await b.page.locator(".index-catalogue").waitFor({ timeout: uiTimeout });

		// "healthz" only appears in the gallery's body, so a hit proves the search
		// reaches through the file rather than the frontmatter metadata.
		await b.page.fill(".index-search", "healthz");
		const first = b.page.locator(".search-result").first();
		await first.waitFor({ timeout: uiTimeout });
		assert.equal(await b.page.locator(".search-result-snippet mark").first().isVisible(), true, "the matched term is marked");
		const href = await first.locator("a").getAttribute("href");
		assert.ok(href.startsWith("/gallery"), `result links to the gallery, got ${href}`);
		assert.equal(await b.page.locator(".index-catalogue").isHidden(), true, "the catalogue is replaced by results");

		await b.page.fill(".index-search", "");
		await b.page.locator(".index-catalogue").waitFor({ state: "visible", timeout: uiTimeout });
		assert.equal(await b.page.locator(".search-result").count(), 0, "clearing restores the catalogue");
		await holdIfAsked();
	} catch (err) {
		await reportFailure(b.page, app.logs());
		throw err;
	}
});

test("consecutive rollback steps keep the step gap", async (t) => {
	const app = await startApp();
	t.after(() => app.stop());
	const b = await startBrowser();
	t.after(() => b.stop());
	try {
		await bootAdmin(b.page, app.base);
		await b.page.goto(app.base + "/gallery");
		const cards = b.page.locator(".rollback-card");
		await cards.first().waitFor({ timeout: uiTimeout });
		assert.equal(await cards.count(), 2, "the gallery carries two rollback steps");
		const first = await cards.nth(0).boundingBox();
		const second = await cards.nth(1).boundingBox();
		assert.ok(
			second.y >= first.y + first.height + 8,
			`rollback steps are spaced, got a ${second.y - (first.y + first.height)}px gap`,
		);
		await holdIfAsked();
	} catch (err) {
		await reportFailure(b.page, app.logs());
		throw err;
	}
});

test("the notes composer: toolbar, help, links, and the hidden-column fix", async (t) => {
	const app = await startApp();
	t.after(() => app.stop());
	const b = await startBrowser();
	t.after(() => b.stop());
	try {
		await bootAdmin(b.page, app.base);
		await b.page.goto(app.base + "/gallery");
		const area = b.page.locator(".notes-textarea");

		// Bold wraps the selection; a second press unwraps it.
		await area.fill("hello");
		await area.evaluate(el => el.setSelectionRange(0, 5));
		await b.page.locator('[data-notes-tool="bold"]').click();
		assert.equal(await area.inputValue(), "**hello**", "bold wraps the selection");
		await b.page.locator('[data-notes-tool="bold"]').click();
		assert.equal(await area.inputValue(), "hello", "a second bold press unwraps");

		// Link inserts [text](url) around the selection and selects the URL.
		await area.evaluate(el => el.setSelectionRange(0, 5));
		await b.page.locator('[data-notes-tool="link"]').click();
		assert.equal(await area.inputValue(), "[hello](url)", "link wraps the selection");

		// Preview makes links open in a new tab, so a run is not navigated away.
		await area.fill("see [grafana](https://grafana.example/d/abc)");
		await b.page.locator('.notes-tab[data-mode="preview"]').click();
		const link = b.page.locator(".notes-preview a");
		assert.equal(await link.getAttribute("target"), "_blank", "preview links open in a new tab");
		assert.equal(await link.getAttribute("rel"), "noopener noreferrer", "preview links carry noopener");
		await b.page.locator('.notes-tab[data-mode="edit"]').click();

		// A pasted note is untrusted: script and event handlers must not survive
		// the preview render, while safe markdown still does.
		await area.fill('<img src=x onerror="window.__pwned=1"><script>window.__pwned=2</script>**safe**');
		await b.page.locator('.notes-tab[data-mode="preview"]').click();
		assert.equal(await b.page.evaluate(() => window.__pwned), undefined, "a malicious note does not execute");
		assert.equal(await b.page.locator(".notes-preview script").count(), 0, "script tags are stripped");
		assert.equal(await b.page.locator(".notes-preview img[onerror]").count(), 0, "event handlers are stripped");
		assert.equal(await b.page.locator(".notes-preview strong").innerText(), "safe", "safe markdown still renders");
		await b.page.locator('.notes-tab[data-mode="edit"]').click();

		// The cheatsheet is non-modal and closes on Esc.
		const sheet = b.page.locator("[data-notes-cheatsheet]");
		await b.page.locator("[data-notes-help]").click();
		assert.equal(await sheet.isVisible(), true, "the cheatsheet opens");
		await b.page.keyboard.press("Escape");
		assert.equal(await sheet.isHidden(), true, "Esc closes the cheatsheet");

		// The overflow exposes Clear.
		await b.page.locator("[data-notes-menu]").click();
		assert.equal(await b.page.locator(".notes-clear").isVisible(), true, "Clear is in the overflow");
		await b.page.keyboard.press("Escape");

		// Drag-to-dismiss: narrowing past the usable floor hides the panel instead
		// of clipping its wrapped toolbar, and the header toggle recalls it.
		const handle = b.page.locator(".notes-resize");
		const box = await handle.boundingBox();
		await b.page.mouse.move(box.x + 2, box.y + box.height / 2);
		await b.page.mouse.down();
		await b.page.mouse.move(box.x + 300, box.y + box.height / 2, { steps: 10 });
		assert.equal(await b.page.locator(".notes-panel").isVisible(), true, "the panel pins at the floor while dragging");
		assert.equal(await b.page.locator(".notes-dismiss-hint").isVisible(), true, "past the floor a release-to-hide cue appears");
		await b.page.mouse.up();
		await b.page.locator(".notes-panel").waitFor({ state: "hidden", timeout: uiTimeout });
		assert.equal(await b.page.locator(".notes-panel").isHidden(), true, "dragging in past the floor dismisses the panel");
		await b.page.locator("[data-notes-toggle]").click();
		await b.page.locator(".notes-panel").waitFor({ state: "visible", timeout: uiTimeout });
		assert.equal(await b.page.locator(".notes-panel").isVisible(), true, "the header toggle recalls it");

		// A dragged width must not survive hiding the panel as a phantom column.
		await b.page.evaluate(() => {
			document.body.style.setProperty("--notes-width", "300px");
			localStorage.setItem("runbooks-notes-width", "300");
		});
		await b.page.locator("[data-notes-toggle]").click();
		await b.page.locator(".notes-panel").waitFor({ state: "hidden", timeout: uiTimeout });
		await b.page.waitForFunction(() => getComputedStyle(document.body).gridTemplateColumns.trim().endsWith("0px"));
		const columns = await b.page.evaluate(() => getComputedStyle(document.body).gridTemplateColumns);
		assert.ok(columns.trim().endsWith("0px"), `hiding notes collapses the third column (got ${columns})`);
	} catch (err) {
		await reportFailure(b.page, app.logs());
		throw err;
	}
});

test("a destructive runbook gates the page behind an acknowledgement", async (t) => {
	const app = await startApp();
	t.after(() => app.stop());
	const b = await startBrowser();
	t.after(() => b.stop());
	try {
		await bootAdmin(b.page, app.base);
		await b.page.goto(app.base + "/destructive-demo");
		const dialog = b.page.locator("[data-ack-dialog]");
		const badge = b.page.locator("[data-ack-badge]");
		await dialog.waitFor({ state: "visible", timeout: uiTimeout });
		assert.equal(await badge.isHidden(), true, "no acknowledgement marker before accepting");
		assert.equal(
			await b.page.locator("[data-ack]").evaluate(el => el.hasAttribute("inert")),
			true,
			"the runbook is inert until acknowledged",
		);
		await b.page.keyboard.press("Escape");
		assert.equal(await dialog.evaluate(el => el.open), true, "Esc does not dismiss the acknowledgement");
		await b.page.locator("[data-ack-accept]").click();
		assert.equal(await dialog.evaluate(el => el.open), false, "accept clears the gate");
		assert.equal(await b.page.locator("[data-ack]").evaluate(el => el.hasAttribute("inert")), false, "the gate is cleared");
		assert.equal(await badge.isVisible(), true, "a reminder is left in the header after accepting");
		await b.page.reload();
		assert.equal(await dialog.evaluate(el => el.open), false, "stays acknowledged within the session");
		assert.equal(await badge.isVisible(), true, "the reminder survives a reload within the session");
	} catch (err) {
		await reportFailure(b.page, app.logs());
		throw err;
	}
});

test("the sidebar nav: the active branch opens, a query narrows, disclosure toggles", async (t) => {
	const app = await startApp();
	t.after(() => app.stop());
	const b = await startBrowser();
	t.after(() => b.stop());
	const page = b.page;
	try {
		await bootAdmin(page, app.base);
		await page.goto(app.base + "/gallery");

		// The tree is a plain server-rendered structure, so drive it through the DOM
		// rather than locator auto-waiting (which retries against detached elements).
		const nav = () =>
			page.evaluate(() => {
				const visible = el => !!el && el.offsetParent !== null;
				const activeLink = document.querySelector("[data-nav-tree] .nav-links a.active");
				const system = activeLink && activeLink.closest(".nav-sys");
				const category = activeLink && activeLink.closest(".nav-cat");
				const toggle = system && system.querySelector("[data-nav-toggle]");
				const results = document.querySelector("[data-nav-results]");
				return {
					activeLinks: document.querySelectorAll("[data-nav-tree] .nav-links a.active").length,
					activeInOpenSystem: !!system && system.classList.contains("open"),
					activeInClosedCategory: !!category && !category.classList.contains("open"),
					activeVisible: visible(activeLink),
					expanded: toggle ? toggle.getAttribute("aria-expanded") : null,
					visibleLinks: [...document.querySelectorAll("[data-nav-tree] .nav-links a")].filter(visible).length,
					resultsHidden: results.hidden,
					resultsText: results.textContent,
					filterValue: document.querySelector(".nav-filter-input").value,
					focused: document.activeElement === document.querySelector(".nav-filter-input"),
				};
			});
		const click = sel => page.evaluate(s => document.querySelector(s).click(), sel);
		const filter = value =>
			page.evaluate(v => {
				const input = document.querySelector(".nav-filter-input");
				input.value = v;
				input.dispatchEvent(new Event("input", { bubbles: true }));
			}, value);

		// The branch owning the active runbook is open, and its link is marked.
		let state = await nav();
		assert.equal(state.activeLinks, 1, "one active link");
		assert.equal(state.activeInOpenSystem, true, "it sits in the open system");
		assert.equal(state.activeInClosedCategory, false, "never in a collapsed category");
		assert.equal(state.expanded, "true", "the active system starts open");
		assert.equal(state.activeVisible, true, "its links are visible");

		// Disclosure: the active system's row collapses and reopens.
		await page.evaluate(() => {
			const toggle = document.querySelector("[data-nav-tree] .nav-links a.active").closest(".nav-sys").querySelector("[data-nav-toggle]");
			toggle.click();
		});
		state = await nav();
		assert.equal(state.expanded, "false", "the row collapses");
		assert.equal(state.activeVisible, false, "its links hide");
		await page.evaluate(() => {
			const toggle = document.querySelector("[data-nav-tree] .nav-links a.active").closest(".nav-sys").querySelector("[data-nav-toggle]");
			toggle.click();
		});
		state = await nav();
		assert.equal(state.activeVisible, true, "reopening restores them");

		// A query overrides collapse across the whole tree, and counts matches.
		await filter("gallery");
		state = await nav();
		assert.equal(state.visibleLinks, 1, "only the match is listed");
		assert.equal(state.resultsHidden, false, "a match line is shown");
		await filter("zzz-no-such-runbook");
		state = await nav();
		assert.equal(state.visibleLinks, 0, "no link survives a miss");
		assert.match(state.resultsText, /^0 /, "the count reports the miss");

		// The clear affordance restores the server's default branch.
		await click("[data-nav-filter-clear]");
		state = await nav();
		assert.equal(state.filterValue, "", "clear empties the field");
		assert.equal(state.resultsHidden, true, "the match line hides again");
		assert.equal(state.activeInOpenSystem, true, "the active branch is back");

		// `/` focuses the filter when no field has focus.
		await page.evaluate(() => document.activeElement.blur());
		await page.keyboard.press("/");
		assert.equal((await nav()).focused, true, "`/` focuses the filter");
	} catch (err) {
		await reportFailure(page, app.logs());
		throw err;
	}
});

test("the rail is the default shell and summons the sidebar at every width", async (t) => {
	const app = await startApp();
	t.after(() => app.stop());
	const b = await startBrowser();
	t.after(() => b.stop());
	const page = b.page;
	const shell = () =>
		page.evaluate(() => ({
			drawer: document.body.dataset.drawer || null,
			expanded: document.querySelector("[data-rail-menu]").getAttribute("aria-expanded"),
			railWidth: document.querySelector(".rail").getBoundingClientRect().width,
			sidebarX: Math.round(document.querySelector(".sidebar").getBoundingClientRect().x),
			scrim: getComputedStyle(document.querySelector(".drawer-scrim")).display,
		}));
	try {
		await bootAdmin(page, app.base);

		// The thin rail is the shell at a wide and a narrow viewport alike.
		for (const width of [1280, 900]) {
			await page.setViewportSize({ width, height: 800 });
			await page.goto(app.base + "/gallery");

			let s = await shell();
			assert.equal(s.railWidth, 56, `the rail is drawn at ${width}px`);
			assert.equal(s.drawer, null, `no drawer starts open at ${width}px`);
			assert.ok(s.sidebarX < 0, `the sidebar starts off-canvas at ${width}px (x=${s.sidebarX})`);
			assert.equal(s.scrim, "none", `no scrim at ${width}px`);

			// Any click on the strip except the brand summons it.
			await page.click(".rail-hit");
			await page.waitForSelector('body[data-drawer="sidebar"]', { timeout: uiTimeout });
			// The drawer slides in (base duration); wait for the transform to settle.
			await page.waitForFunction(() => document.querySelector(".sidebar").getBoundingClientRect().x === 0, null, { timeout: uiTimeout });
			s = await shell();
			assert.equal(s.sidebarX, 0, `the sidebar covers the rail at ${width}px`);
			assert.equal(s.expanded, "true", "the rail reports expanded");
			assert.notEqual(s.scrim, "none", "the scrim is shown");

			await page.keyboard.press("Escape");
			await page.waitForFunction(() => !document.body.dataset.drawer && document.querySelector(".sidebar").getBoundingClientRect().x < 0, null, { timeout: uiTimeout });
			assert.ok((await shell()).sidebarX < 0, "Esc parks the sidebar again");
		}

		// The brand keeps its own meaning: it links home, it does not open the drawer.
		await page.click(".rail-brand");
		await page.waitForURL((url) => new URL(url).pathname === "/", { timeout: uiTimeout });
		assert.equal((await shell()).drawer, null, "the brand does not open the drawer");
	} catch (err) {
		await reportFailure(page, app.logs());
		throw err;
	}
});
