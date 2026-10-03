// SPDX-License-Identifier: FSL-1.1-MIT

// Styleguide E2E: renders the design system inside the app shell and captures
// each section, so a visual change can be eyeballed from one run. Screenshots are
// the point — there is little to assert beyond the page rendering.
import { test } from "node:test";
import assert from "node:assert/strict";
import { startApp, startBrowser, bootAdmin, snap, reportFailure, holdIfAsked } from "./harness.mjs";

const sections = ["", "#step", "#badge", "#field", "#layouts", "#language", "#code", "#appearance", "#sidebar-nav"];

test("the styleguide renders its sections", async (t) => {
	const app = await startApp();
	t.after(() => app.stop());
	const b = await startBrowser();
	t.after(() => b.stop());
	try {
		await bootAdmin(b.page, app.base);
		for (const section of sections) {
			await b.page.goto(app.base + "/styleguide" + section);
			await snap(b.page, "styleguide" + (section ? "-" + section.slice(1) : ""));
		}
		// Regression: the sidebar-nav specimen renders the component's own link
		// layout. Keying it to .sidebar left the links bare, underlined and touching.
		const links = b.page.locator(".sg-sidebar .nav-links a");
		await links.first().waitFor();
		assert.equal(
			await links.first().evaluate(el => getComputedStyle(el).textDecorationLine),
			"none",
			"specimen links are not underlined",
		);
		const boxes = await links.evaluateAll(els => els.slice(0, 2).map(el => el.getBoundingClientRect()));
		assert.ok(boxes[1].top - (boxes[0].top + boxes[0].height) >= 1, "specimen links are spaced, not touching");

		// Regression: the appearance specimen is an inline, inert example, not a
		// second floating panel wired to the real one (which doubled on click).
		await b.page.goto(app.base + "/styleguide#appearance");
		const specimenPanel = b.page.locator(".sg-appearance-panel");
		await specimenPanel.waitFor();
		assert.equal(
			await specimenPanel.evaluate(el => getComputedStyle(el).position),
			"static",
			"the specimen panel is inline, not a fixed overlay",
		);
		const realPanel = b.page.locator("[data-appearance-panel]");
		assert.equal(await realPanel.isHidden(), true, "the real panel starts hidden");
		await b.page.locator("[data-appearance-specimen] [data-appearance-toggle]").click();
		assert.equal(await realPanel.isHidden(), true, "the specimen trigger does not open the real panel");

		await b.page.goto(app.base + "/styleguide#dialog");
		await b.page.locator('[data-dialog-open="sg-ack"]').click();
		await b.page.locator("#sg-ack").waitFor({ state: "visible" });
		await snap(b.page, "styleguide-ack");
		await holdIfAsked();
	} catch (err) {
		await reportFailure(b.page, app.logs());
		throw err;
	}
});
