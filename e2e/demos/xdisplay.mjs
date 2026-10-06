// SPDX-License-Identifier: FSL-1.1-MIT

// The demo browser runs on its own X server.
//
// Chromium is driven and asserted through Playwright, exactly as before, but the
// pointer is not Playwright's and not injected: it is the real X cursor, moved
// and clicked with xdotool, and the video is the display itself captured at 60fps
// with ffmpeg's x11grab. The X server is a private Xvfb, so nothing here touches
// the host session (Wayland or X11), and the pointer, its hot spots and the
// browser's own hover states are all genuine rather than simulated.
//
// The window keeps its chrome: with no window manager inside Xvfb, fullscreen is a
// no-op, so the X screen is made slightly taller than the page and the capture is
// the page rect only. Where that rect starts is measured, not assumed — see
// calibrate.
//
// System requirements: Xvfb and xdotool, ffmpeg, and Chromium.
import { execFileSync, spawn } from "node:child_process";
import { existsSync } from "node:fs";
import { chromium } from "playwright-core";

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
const sleepSync = (ms) => Atomics.wait(new Int32Array(new SharedArrayBuffer(4)), 0, 0, ms);

function requireBin(bin, hint) {
	try {
		execFileSync("which", [bin], { stdio: "ignore" });
	} catch {
		throw new Error(`${bin} is required to record the browser clip: ${hint}`);
	}
}

// A display number nothing else is using: Xvfb leaves a lock file per display.
function freeDisplay() {
	for (let n = 99; n < 130; n += 1) if (!existsSync(`/tmp/.X${n}-lock`)) return n;
	throw new Error("no free X display number");
}

async function startXvfb(n, width, height) {
	// The screen geometry is three argv entries, not one: Xvfb reads the screen
	// number and the geometry separately.
	const child = spawn("Xvfb", [`:${n}`, "-screen", "0", `${width}x${height}x24`, "-nolisten", "tcp"], {
		stdio: ["ignore", "ignore", "pipe"],
	});
	let logs = "";
	child.stderr.on("data", (chunk) => (logs += chunk));
	let exited = null;
	child.on("exit", (code, signal) => (exited = `code ${code} signal ${signal}`));
	const env = { ...process.env, DISPLAY: `:${n}` };
	for (let i = 0; i < 80; i++) {
		if (exited) break; // fail with its output rather than after the timeout
		try {
			execFileSync("xdotool", ["getdisplaygeometry"], { env, stdio: "ignore" });
			return child;
		} catch {
			sleepSync(100);
		}
	}
	child.kill("SIGKILL");
	throw new Error(`Xvfb :${n} did not come up${exited ? ` (exited: ${exited})` : ""}:\n${logs}`);
}

// startDisplay boots Xvfb + Chromium on it and returns the handles a clip needs.
// size is the page viewport; the X screen is taller so the window's chrome still
// fits on screen and can be cropped off.
export async function startDisplay({ size = { width: 1280, height: 800 }, chromeAllowance = 150, url } = {}) {
	requireBin("Xvfb", "pacman -S xorg-server-xvfb");
	requireBin("xdotool", "pacman -S xdotool");

	const display = `:${freeDisplay()}`;
	const xvfb = await startXvfb(
		display.slice(1),
		size.width,
		size.height + chromeAllowance,
	);
	const env = { ...process.env, DISPLAY: display, XCURSOR_THEME: "Adwaita", XCURSOR_SIZE: "24" };
	const x = (...args) => execFileSync("xdotool", args, { env }).toString().trim();

	const browser = await chromium.launch({
		executablePath: process.env.CHROMIUM || "/usr/bin/chromium",
		headless: false,
		env,
		args: [
			"--no-sandbox",
			"--ozone-platform=x11", // without this Chromium renders offscreen and the display stays black
			"--disable-gpu", // no GPU in Xvfb; software rasterisation is what composites to X
			"--window-position=0,0",
			`--window-size=${size.width},${size.height}`,
			"--no-first-run",
			"--no-default-browser-check",
		],
	});
	const context = await browser.newContext({ viewport: size });
	const page = await context.newPage();
	// Track the real pointer's page coordinates on every navigation. This is an
	// init script rather than a one-shot evaluate because the reader reloads the
	// page: a listener installed once would be dropped by the reload, leaving
	// `__demoPointer` null so every travel jumps instantly to its target.
	await page.addInitScript(() => {
		window.__demoPointer = null;
		addEventListener("mousemove", (e) => (window.__demoPointer = { x: e.clientX, y: e.clientY }), true);
	});
	if (url) await page.goto(url);

	// The page rect inside the X screen, measured by asking the page where it
	// thinks the pointer is after putting the pointer somewhere known. Assumed
	// geometry (chrome height, borders) is exactly the kind of thing that drifts.
	let origin = null;
	async function calibrate() {
		const at = await page.evaluate(() => ({ x: Math.round(innerWidth / 2), y: Math.round(innerHeight / 2) }));
		for (const probe of [at, { x: at.x + 40, y: at.y + 40 }]) {
			x("mousemove", String(probe.x), String(probe.y));
			await sleep(200);
			const seen = await page.evaluate(() => window.__demoPointer);
			if (!seen) continue;
			origin = { x: probe.x - seen.x, y: probe.y - seen.y };
			await sleep(120);
			return origin;
		}
		throw new Error("the page never saw a real pointer event: is the browser window on screen?");
	}
	await calibrate();

	// Root coordinates are what xdotool takes; page coordinates are what the
	// bounding boxes give.
	const toRoot = (p) => ({ x: Math.round(p.x + origin.x), y: Math.round(p.y + origin.y) });
	// Keep the pointer inside the page: on the root window the X default cursor is
	// a cross, which would show up in the video.
	const clamp = (root) => ({
		x: Math.min(Math.max(root.x, origin.x + 1), origin.x + size.width - 2),
		y: Math.min(Math.max(root.y, origin.y + 1), origin.y + size.height - 2),
	});
	let lastPage = null;
	const at = (p) => {
		lastPage = p;
		x("mousemove", String(clamp(toRoot(p)).x), String(clamp(toRoot(p)).y));
	};

	// move takes page coordinates and travels, so the pointer is seen to arrive
	// rather than teleport. The easing and the pacing are what make it read as a
	// cursor rather than an edit: at rest at both ends, and slower over distance.
	async function move(pagePoint, duration) {
		// Prefer what the page actually saw; fall back to the last commanded point so
		// a lost listener degrades to a short travel rather than a teleport.
		const from = (await page.evaluate(() => window.__demoPointer).catch(() => null)) ?? lastPage;
		const distance = from ? Math.hypot(pagePoint.x - from.x, pagePoint.y - from.y) : 0;
		const ms = duration ?? Math.min(900, 280 + distance * 0.8);
		const steps = Math.max(6, Math.round(ms / 24));
		for (let i = 1; i <= steps; i++) {
			const t = i / steps;
			const ease = t * t * (3 - 2 * t);
			const p = from
				? { x: from.x + (pagePoint.x - from.x) * ease, y: from.y + (pagePoint.y - from.y) * ease }
				: pagePoint;
			at(p);
			await sleep(24);
		}
		at(pagePoint);
	}

	// click is a real press and release with a visible hold, so :active and the
	// cursor's own pressed state land on camera.
	async function click({ hold = 80 } = {}) {
		x("mousedown", "1");
		await sleep(hold);
		x("mouseup", "1");
	}

	let capture = null;
	async function startCapture(outPath) {
		capture = spawn(
			"ffmpeg",
			[
				"-y", "-loglevel", "error",
				"-f", "x11grab", "-framerate", "60",
				"-video_size", `${size.width}x${size.height}`,
				"-i", `${display}+${origin.x},${origin.y}`,
				"-draw_mouse", "1",
				"-c:v", "libx264", "-preset", "slow", "-crf", "16",
				"-pix_fmt", "yuv420p", "-movflags", "+faststart",
				outPath,
			],
			{ env, stdio: ["pipe", "ignore", "pipe"] },
		);
		let logs = "";
		capture.stderr.on("data", (chunk) => (logs += chunk));
		await sleep(600); // let the encoder open the display before the first gesture
		return {
			async stop() {
				if (!capture) return;
				capture.stdin.write("q"); // ffmpeg's clean exit: it finalises the file
				await new Promise((resolve) => capture.on("exit", resolve));
				if (logs.trim()) console.error(logs.trim());
				capture = null;
			},
		};
	}

	return {
		page,
		display,
		origin,
		at,
		move,
		click,
		startCapture,
		async stop() {
			await capture?.stop?.();
			await browser.close().catch(() => {});
			xvfb.kill("SIGTERM");
		},
	};
}
