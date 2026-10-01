// Styleguide E2E: renders the design system inside the app shell and captures
// each section, so a visual change can be eyeballed from one run. Screenshots are
// the point — there is little to assert beyond the page rendering.
import { test } from "node:test";
import { startApp, startBrowser, bootAdmin, snap, reportFailure, holdIfAsked } from "./harness.mjs";

const sections = ["", "#step", "#badge", "#field", "#layouts", "#code", "#appearance"];

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
		await holdIfAsked();
	} catch (err) {
		await reportFailure(b.page, app.logs());
		throw err;
	}
});
