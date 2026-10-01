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

		// The destructive Clear confirmation is a real dialog, not window.confirm().
		await b.page.locator(".notes-clear").click();
		await b.page.locator("dialog.dialog").waitFor({ timeout: uiTimeout });
		await snap(b.page, "dialog");
		await b.page.locator("dialog.dialog .btn-ghost").click();

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

test("a destructive runbook gates the page behind an acknowledgement", async (t) => {
	const app = await startApp();
	t.after(() => app.stop());
	const b = await startBrowser();
	t.after(() => b.stop());
	try {
		await bootAdmin(b.page, app.base);
		await b.page.goto(app.base + "/destructive-demo");
		const dialog = b.page.locator("[data-ack-dialog]");
		await dialog.waitFor({ state: "visible", timeout: uiTimeout });
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
		await b.page.reload();
		assert.equal(await dialog.evaluate(el => el.open), false, "stays acknowledged within the session");
	} catch (err) {
		await reportFailure(b.page, app.logs());
		throw err;
	}
});
