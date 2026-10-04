// SPDX-License-Identifier: FSL-1.1-MIT

// Docs-site E2E: a public, no-database instance serving a sections-based content
// root. Covers the fold's productised surface — sections vs numbered steps, the
// page lead, prev/next, the inline glossary popup, and the crawler surface
// (canonical/OpenGraph, sitemap, robots) — none of which the styleguide can
// exercise as real pages.
import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import {
	uiTimeout,
	navTimeout,
	startApp,
	startBrowser,
	snap,
	reportFailure,
	holdIfAsked,
} from "./harness.mjs";

const repoRoot = dirname(dirname(fileURLToPath(import.meta.url)));
const docsDir = join(repoRoot, "e2e", "testdata", "docs");

test("the public docs: sections, lead, prev/next and the crawler surface", async (t) => {
	const app = await startApp({
		identity: false,
		contentDir: docsDir,
		publicURL: true,
		env: { SITE_DESCRIPTION: "Runbooks documentation" },
	});
	t.after(() => app.stop());
	const b = await startBrowser();
	t.after(() => b.stop());
	try {
		await b.page.goto(app.base + "/configuration", { waitUntil: "load" });
		await b.page.locator(".section").first().waitFor({ timeout: uiTimeout });

		// The docs tree groups by audience, in _meta.yml order.
		assert.deepEqual(
			await b.page.locator(".nav-sys-name").allTextContents(),
			["Get started", "Guides", "Reference", "Operations"],
			"sidebar groups render in order",
		);

		// A sections page: every ## is an anchored, unnumbered section with no
		// completion affordance, and the intro prose renders as a lead.
		assert.ok((await b.page.locator(".section").count()) >= 2, "configuration has sections");
		assert.equal(await b.page.locator(".section .step-check").count(), 0, "sections carry no done tick");
		assert.equal(await b.page.locator(".toc-num").count(), 0, "sections are not numbered in the contents");
		assert.ok(
			await b.page.locator(".section").first().getAttribute("id"),
			"a section is anchored",
		);
		assert.equal(
			await b.page.locator(".section-lead").first().isVisible(),
			true,
			"the page lead renders",
		);
		assert.equal(
			await b.page.locator('[data-steps-action="expand"]').count(),
			0,
			"no step controls on a sections page",
		);

		// Prev/next follow sidebar order and are shown on a sections page.
		const next = b.page.locator(".page-nav-next");
		await next.waitFor({ timeout: uiTimeout });
		assert.ok((await next.getAttribute("href")).startsWith("/"), "next links to a page");

		// Crawler surface: canonical and OpenGraph derive from PUBLIC_URL.
		assert.equal(
			await b.page.locator('link[rel="canonical"]').getAttribute("href"),
			app.base + "/configuration",
			"canonical URL",
		);
		assert.equal(
			await b.page.locator('meta[property="og:type"]').getAttribute("content"),
			"article",
			"og:type",
		);

		// The sitemap and robots are generated from the same content groups.
		const sitemap = await (await fetch(app.base + "/sitemap.xml")).text();
		assert.ok(sitemap.includes(app.base + "/configuration"), "sitemap lists the page");
		assert.ok(sitemap.includes(app.base + "/"), "sitemap lists the index");
		const robots = await (await fetch(app.base + "/robots.txt")).text();
		assert.match(robots, /Sitemap:/, "robots points at the sitemap");

		// The agent surface still resolves on the public instance.
		assert.equal((await fetch(app.base + "/llms.txt")).status, 200, "llms.txt");
		assert.equal((await fetch(app.base + "/configuration.md")).status, 200, "raw markdown");

		// Opt-in full-page captures of the docs tree for visual review
		// (E2E_SCREENSHOT_DIR=… writes them).
		if (process.env.E2E_SCREENSHOT_DIR) {
			// Hide the floating notes panel so it does not occlude the page.
			await b.page.evaluate(() => localStorage.setItem("runbooks-notes", "off"));
			for (const slug of ["overview", "install", "writing-runbooks", "configuration", "deployment", "identity", "agent-access"]) {
				await b.page.goto(`${app.base}/${slug}`, { waitUntil: "load" });
				await b.page.locator(".section, .step-card").first().waitFor({ timeout: uiTimeout });
				// The page scrolls inside .content, so fullPage alone captures only the
				// viewport; let the container grow for the capture.
				await b.page.evaluate(() => {
					for (const sel of [".content", ".main"]) {
						const el = document.querySelector(sel);
						if (el) Object.assign(el.style, { height: "auto", maxHeight: "none", overflow: "visible" });
					}
				});
				await snap(b.page, `docs-${slug}`, { fullPage: true });
			}
			await b.page.evaluate(() => localStorage.setItem("runbooks-theme", "light"));
			await b.page.goto(app.base + "/writing-runbooks", { waitUntil: "load" });
			await snap(b.page, "docs-writing-runbooks-light", { fullPage: true });
			await b.page.evaluate(() => localStorage.removeItem("runbooks-theme"));
			await b.page.setViewportSize({ width: 390, height: 844 });
			await b.page.goto(app.base + "/overview", { waitUntil: "load" });
			await snap(b.page, "docs-overview-mobile", { fullPage: true });
		} else {
			await snap(b.page, "docs-sections");
		}
		await holdIfAsked();
	} catch (err) {
		await reportFailure(b.page, app.logs());
		throw err;
	}
});

test("a mixed page: sections and numbered steps interleave, and glossary terms pop", async (t) => {
	const dir = mkdtempSync(join(tmpdir(), "runbooks-docs-"));
	t.after(() => rmSync(dir, { recursive: true, force: true }));
	writeFileSync(
		join(dir, "_glossary.yml"),
		[
			"- term: MTS",
			"  expansion: Multi-Threaded Replica",
			"  description: A MySQL replica applying transactions in parallel workers.",
			"",
		].join("\n"),
	);
	writeFileSync(
		join(dir, "mixed.md"),
		[
			"---",
			"title: Mixed",
			"slug: mixed",
			"layout: sections",
			"---",
			"",
			"An intro mentioning MTS in passing.",
			"",
			"## Reference",
			"",
			"Reference prose.",
			"",
			"<!-- steps -->",
			"",
			"## Run it",
			"",
			"Do the thing.",
			"",
			"<!-- sections -->",
			"",
			"## Afterwards",
			"",
			"Trailing prose.",
			"",
		].join("\n"),
	);
	writeFileSync(
		join(dir, "numbered.md"),
		["---", "title: Numbered", "slug: numbered", "---", "", "## First step", "", "Prose.", ""].join("\n"),
	);

	const app = await startApp({ identity: false, contentDir: dir, publicURL: true });
	t.after(() => app.stop());
	const b = await startBrowser();
	t.after(() => b.stop());
	try {
		await b.page.goto(app.base + "/mixed", { waitUntil: "load" });
		await b.page.locator(".section").first().waitFor({ timeout: uiTimeout });

		// Two section regions with a single numbered step between them; the step
		// numbers among itself, and the sections stay unnumbered and anchored.
		assert.equal(await b.page.locator(".section").count(), 2, "two section regions");
		assert.equal(await b.page.locator(".step-card").count(), 1, "one step region");
		assert.equal(await b.page.locator(".step-num").first().textContent(), "1", "steps number among themselves");
		assert.ok(await b.page.locator(".section").first().getAttribute("id"), "section anchored");
		assert.equal(await b.page.locator(".page-nav").count(), 1, "a sections page has prev/next");

		// The glossary marks a term in prose (never code) and pops on hover.
		const term = b.page.locator(".glossary-term").first();
		await term.waitFor({ timeout: uiTimeout });
		assert.equal(await term.textContent(), "MTS", "the configured term is marked");
		await term.hover();
		const popup = b.page.locator(".glossary-popup.open");
		await popup.waitFor({ timeout: uiTimeout });
		assert.equal(
			await popup.locator(".glossary-popup-exp").textContent(),
			"Multi-Threaded Replica",
			"the expansion is shown",
		);

		// A numbered runbook (no layout) has no sections and no prev/next.
		await b.page.goto(app.base + "/numbered", { waitUntil: "load" });
		await b.page.locator(".step-card").first().waitFor({ timeout: uiTimeout });
		assert.equal(await b.page.locator(".section").count(), 0, "no sections on a numbered page");
		assert.equal(await b.page.locator(".page-nav").count(), 0, "a numbered runbook has no prev/next");
		await snap(b.page, "docs-mixed");
		await holdIfAsked();
	} catch (err) {
		await reportFailure(b.page, app.logs());
		throw err;
	}
});

test("an identity-gated instance is not crawlable", async (t) => {
	const app = await startApp({ identity: true, publicURL: true });
	t.after(() => app.stop());
	try {
		const robots = await (await fetch(app.base + "/robots.txt")).text();
		assert.match(robots, /Disallow: \//, "a private instance disallows crawling");
		const sitemap = await fetch(app.base + "/sitemap.xml");
		assert.notEqual(sitemap.status, 200, "the sitemap sits behind the read gate");
	} catch (err) {
		console.error("--- app log ---\n" + app.logs());
		throw err;
	}
});
