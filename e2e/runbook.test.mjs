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
		await b.page.locator(".rollback-card").scrollIntoViewIfNeeded();
		await snap(b.page, "runbook-light-rollback");
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
