// SPDX-License-Identifier: FSL-1.1-MIT

// Records the landing-page "reader" clip: the reader interactions on a real
// runbook page, driven by script rather than a human. It boots a public
// (identity off) instance, so the demo shows the deployment shape the docs
// describe, and it asserts every interaction it films — a UI change breaks the
// recording instead of shipping a stale clip.
//
// The browser runs on its own Xvfb display (xdisplay.mjs): the pointer is the real
// X cursor moved with xdotool and the video is the display at 60fps. Nothing is
// injected into the page and nothing simulates the pointer, so hover, :active and
// the cursor itself are genuine. Playwright keeps the jobs it is good at — driving
// the page through selectors, typing, and asserting what the clip shows.
//
// Output: DEMO_OUT/reader.mp4. Pacing is deliberately slow; there is no audio.
import assert from "node:assert/strict";
import { mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { startApp, uiTimeout } from "../harness.mjs";
import { startDisplay } from "./xdisplay.mjs";

const repoRoot = dirname(dirname(dirname(fileURLToPath(import.meta.url))));
const outDir = process.env.DEMO_OUT || join(repoRoot, "dist", "demos");
const size = { width: 1280, height: 800 };

const beat = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

// The runbook page scrolls inside .runbook-scroll, not the window: window.scrollY
// stays 0 there, so anything that waits on it is a no-op that silently passes and
// lets the next boundingBox be read mid-animation.
const scroller = (page) => page.locator(".runbook-scroll");
const scrollOffset = (page) => scroller(page).evaluate((el) => Math.round(el.scrollTop));

async function settleScroll(page) {
	await beat(120);
	let last = await scrollOffset(page);
	for (let i = 0; i < 60; i++) {
		await beat(60);
		const now = await scrollOffset(page);
		if (now === last) return;
		last = now;
	}
}

async function smoothScrollTo(page, locator, block = "center") {
	await locator.evaluate((el, block) => el.scrollIntoView({ behavior: "smooth", block }), block);
	await settleScroll(page);
}

async function scrollToTop(page) {
	await scroller(page).evaluate((el) => el.scrollTo({ top: 0, behavior: "smooth" }));
	await settleScroll(page);
}

// reachable asserts the point will actually hit the target. xdotool clicks are as
// blind as mouse.down/up was: a target sitting under the sticky vars bar would be
// clicked as whatever is on top of it — silently doing the wrong thing. Checking
// here, immediately before the press, is what catches a page that moved during the
// pointer's travel.
async function reachable(page, locator, point) {
	const handle = await locator.elementHandle();
	const cover = await page.evaluate(
		([p, el]) => {
			const hit = document.elementFromPoint(p.x, p.y);
			const name = (n) => (n ? `${n.tagName.toLowerCase()}.${String(n.className).split(" ").join(".")}` : "null");
			return {
				ok: !!hit && (hit === el || el.contains(hit) || hit.contains(el)),
				stack: document.elementsFromPoint(p.x, p.y).slice(0, 4).map(name).join(" < "),
				target: name(el),
				rect: el.getBoundingClientRect().toJSON(),
				html: String(el.outerHTML).slice(0, 120),
			};
		},
		[point, handle],
	);
	assert.ok(
		cover.ok,
		`something covers the target at ${Math.round(point.x)},${Math.round(point.y)}: ` +
			`stack=[${cover.stack}] target=${cover.target} rect=${JSON.stringify(cover.rect)} html=${cover.html}`,
	);
}

// pointAt scrolls the target in only if it is not already visible, then travels
// the pointer onto it. "nearest" matters: centring an element that is already on
// screen scrolls the page for nothing, which reads as the frame lurching.
async function pointAt(ctx, locator, { block = "nearest", pause = 190, duration } = {}) {
	await smoothScrollTo(ctx.page, locator, block);
	const box = await locator.boundingBox();
	assert.ok(box, "the pointer target is on screen");
	const point = { x: box.x + box.width / 2, y: box.y + box.height / 2 };
	await ctx.xd.move(point, duration);
	await beat(pause);
	return point;
}

async function press(ctx, locator, opts) {
	const point = await pointAt(ctx, locator, opts);
	await reachable(ctx.page, locator, point);
	await ctx.xd.click();
}

async function main() {
	mkdirSync(outDir, { recursive: true });

	const app = await startApp({ identity: false });
	const xd = await startDisplay({ size, url: app.base + "/gallery" });
	let capture = null;
	try {
		const { page } = xd;
		// The panel is a distraction in this clip and the preference is applied
		// before paint; the reader's own toggle is elsewhere. One reload applies it,
		// before the capture starts.
		await page.evaluate(() => localStorage.setItem("runbooks-notes", "off"));
		await page.reload();
		await page.locator(".step-card").first().waitFor({ timeout: uiTimeout });

		// Park the pointer in the reading column: it is on screen from the first
		// frame, and over the root window X draws a cross rather than an arrow.
		await xd.at({ x: 430, y: 620 });
		await beat(600);

		capture = await xd.startCapture(join(outDir, "reader.mp4"));
		await beat(900); // the establishing moment

		const ctx = { page, xd };

		// Inputs: the gallery's fields live in the editor dialog. Typing HOST
		// substitutes it into every block on the page as it goes.
		await press(ctx, page.locator("[data-vars-open]"));
		const dialog = page.locator("[data-vars-dialog]");
		await dialog.waitFor({ state: "visible", timeout: uiTimeout });
		await beat(700);
		const host = page.locator('.var-input[data-var="HOST"]');
		await press(ctx, host);
		await host.pressSequentially("db-2.prod.internal", { delay: 40 });
		await beat(500);
		assert.equal(await page.locator("[data-vars-set]").textContent(), "1 set", "the summary counts the set input");
		await press(ctx, page.locator("[data-vars-done]"));
		await dialog.waitFor({ state: "hidden", timeout: uiTimeout });
		await beat(300);

		// Ticking every block in a step ticks the step itself. The default reading
		// preference opens only the first step, so expand this one first.
		const codeStep = page.locator("#code-blocks");
		await smoothScrollTo(page, codeStep, "center");
		await press(ctx, codeStep.locator(".step-toggle"));
		await beat(500);
		const checks = codeStep.locator(".block-check");
		const count = await checks.count();
		assert.ok(count > 0, "the code step has labelled blocks to tick");
		for (let i = 0; i < count; i++) {
			// Centre each box before ticking it. The step is taller than the viewport,
			// so with "nearest" the lower boxes would be ticked at the bottom edge.
			await press(ctx, checks.nth(i), { block: "center", pause: 160 });
			await beat(380);
		}
		assert.equal(
			await codeStep.evaluate((el) => el.classList.contains("done")),
			true,
			"the step rolls up when all its blocks are ticked",
		);
		await beat(600);

		// The completion timeline: ticking wrote timestamped markdown into the
		// notes, so recall the panel and show it rendered.
		await press(ctx, page.locator("[data-notes-toggle]"));
		const notes = page.locator(".notes-panel");
		await notes.waitFor({ state: "visible", timeout: uiTimeout });
		await beat(900);
		await press(ctx, page.locator('.notes-tab[data-mode="preview"]'));
		await beat(2600);
		const entries = await page.locator(".notes-preview").textContent();
		assert.match(entries, /Block 2\.\d+: /, "the timeline recorded the blocks");
		await press(ctx, page.locator(".notes-close"));
		await notes.waitFor({ state: "hidden", timeout: uiTimeout });
		await beat(300);

		// A decision callout: a branch between two procedures.
		const branchStep = page.locator("#notices-and-branch");
		await smoothScrollTo(page, branchStep, "center");
		await press(ctx, branchStep.locator(".step-toggle"));
		await beat(400);
		await smoothScrollTo(page, page.locator(".notice-branch").first());
		await beat(900);

		// Zen: one step at a time, with the chrome gone. The toolbar is not sticky,
		// so it only clears the sticky vars bar at the top of the page.
		await scrollToTop(page);
		await beat(400);
		await press(ctx, page.locator('[data-zen-action="enter"]'));
		await page.locator(".zen-bar").waitFor({ state: "visible", timeout: uiTimeout });
		await beat(800);
		await press(ctx, page.locator('[data-zen-action="next"]'));
		await beat(1000);
		await press(ctx, page.locator('[data-zen-action="exit"]'));
		await beat(600);

		await capture.stop();
		capture = null;
		console.log(join(outDir, "reader.mp4"));
	} finally {
		await capture?.stop().catch(() => {});
		await xd.stop();
		await app.stop();
	}
}

await main();
