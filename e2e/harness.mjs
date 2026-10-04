// SPDX-License-Identifier: FSL-1.1-MIT

// Shared browser E2E harness. Boots a throwaway app instance (empty SQLite DB,
// random port, identity on) and a fresh Chromium with a CDP virtual authenticator
// shaped like a synced passkey. Each suite owns one concern and boots its own;
// the helpers here are the shared plumbing.
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import net from "node:net";
import { chromium } from "playwright-core";

export const bootstrapToken = "e2e-bootstrap-token";
export const recoveryToken = "e2e-recovery-token";

const repoRoot = dirname(dirname(fileURLToPath(import.meta.url)));
const binary = join(repoRoot, "runbooks");

const isTruthy = (v) => v === "1" || v === "true" || v === "yes";
const inspect = isTruthy(process.env.E2E_INSPECT);
const headed = isTruthy(process.env.E2E_HEADED) || inspect;
const slowMo = Number(process.env.E2E_SLOWMO || (headed ? 600 : 0));
const holdMs = Number(process.env.E2E_HOLD_MS || 0);
export const navTimeout = inspect ? 0 : 10000;
export const uiTimeout = inspect ? 0 : 5000;

const shotDir = process.env.E2E_SCREENSHOT_DIR;

export const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
export const atPath = (base, pathname) => (url) => url.origin === new URL(base).origin && url.pathname === pathname;

// snap saves a screenshot when E2E_SCREENSHOT_DIR is set — for eyeballing UI
// changes the assertions cannot judge. Pass { fullPage: true } to capture the
// whole page rather than the viewport.
export async function snap(page, name, opts = {}) {
	if (!shotDir) return;
	mkdirSync(shotDir, { recursive: true });
	await page.screenshot({ path: join(shotDir, name + ".png"), ...opts });
}

function freePort() {
	return new Promise((resolve, reject) => {
		const srv = net.createServer();
		srv.on("error", reject);
		srv.listen(0, "127.0.0.1", () => {
			const { port } = srv.address();
			srv.close(() => resolve(port));
		});
	});
}

function findChromium() {
	const candidates = [process.env.CHROMIUM, "/usr/bin/chromium", "/usr/bin/chromium-browser", "/usr/bin/google-chrome"];
	for (const path of candidates) {
		if (path && existsSync(path)) return path;
	}
	throw new Error("no Chromium found; install chromium or set CHROMIUM=/path/to/chromium");
}

async function waitForReady(base, child, logs, readyPath) {
	const deadline = Date.now() + 15000;
	while (Date.now() < deadline) {
		if (child.exitCode !== null) throw new Error(`app exited early:\n${logs()}`);
		try {
			const res = await fetch(base + readyPath);
			if (res.ok) return;
		} catch {
			// not up yet
		}
		await sleep(150);
	}
	throw new Error(`app not ready after 15s:\n${logs()}`);
}

// startApp boots the built binary on a free port. Options:
//   identity   — SQLite identity on (default true); false boots a public,
//                no-database instance (a docs deployment).
//   contentDir — CONTENT_DIR override (e.g. the repo's docs/ tree).
//   publicURL  — set PUBLIC_URL to the instance base (canonical, sitemap).
//   env        — extra environment variables.
export async function startApp({ identity = true, contentDir, publicURL = false, env = {} } = {}) {
	const port = await freePort();
	const base = `http://localhost:${port}`;
	const dir = mkdtempSync(join(tmpdir(), "runbooks-e2e-"));
	let logs = "";
	const childEnv = { ...process.env, PORT: String(port), ...env };
	if (contentDir) childEnv.CONTENT_DIR = contentDir;
	if (publicURL) childEnv.PUBLIC_URL = base;
	if (identity) {
		Object.assign(childEnv, {
			IDENTITY_DB_DRIVER: "sqlite",
			IDENTITY_DB_DSN: `file:${join(dir, "identity.db")}`,
			IDENTITY_PUBLIC_URL: base,
			IDENTITY_BOOTSTRAP_TOKEN: bootstrapToken,
			IDENTITY_RECOVERY_TOKEN: recoveryToken,
			IDENTITY_SECURE_COOKIES: "false",
		});
	} else {
		// The dev mise.toml exports identity vars; clear them so a public docs
		// instance really boots with no database and no gating.
		for (const key of Object.keys(childEnv)) {
			if (key.startsWith("IDENTITY_")) delete childEnv[key];
		}
	}
	const child = spawn(binary, [], {
		cwd: repoRoot,
		env: childEnv,
		stdio: ["ignore", "pipe", "pipe"],
	});
	child.stdout.on("data", (chunk) => (logs += chunk));
	child.stderr.on("data", (chunk) => (logs += chunk));
	try {
		await waitForReady(base, child, () => logs, identity ? "/setup" : "/healthz");
	} catch (err) {
		child.kill("SIGKILL");
		rmSync(dir, { recursive: true, force: true });
		throw err;
	}
	return {
		base,
		logs: () => logs,
		async stop() {
			child.kill("SIGTERM");
			await sleep(500);
			if (child.exitCode === null) child.kill("SIGKILL");
			rmSync(dir, { recursive: true, force: true });
		},
	};
}

// startBrowser opens a fresh browser (no extensions, so no password manager)
// with a virtual authenticator shaped like a synced passkey.
export async function startBrowser() {
	const browser = await chromium.launch({
		executablePath: findChromium(),
		headless: !headed,
		slowMo,
		args: ["--no-sandbox"],
	});
	const context = await browser.newContext();
	const page = await context.newPage();
	const cdp = await context.newCDPSession(page);
	await cdp.send("WebAuthn.enable");
	const { authenticatorId } = await cdp.send("WebAuthn.addVirtualAuthenticator", {
		options: {
			protocol: "ctap2",
			transport: "internal",
			hasResidentKey: true,
			hasUserVerification: true,
			// Chromium names these defaultBackup*, not hasBackup* (which it silently ignores).
			defaultBackupEligibility: true,
			defaultBackupState: true,
			isUserVerified: true,
			automaticPresenceSimulation: true,
		},
	});
	return {
		page,
		cdp,
		authenticatorId,
		async stop() {
			if (browser.isConnected()) await browser.close();
		},
	};
}

// pauseAt opens the Playwright Inspector at a step, in E2E_INSPECT mode.
export const pauseAt = (page) => (inspect ? page.pause() : Promise.resolve());

export async function bootAdmin(page, base) {
	await page.goto(base + "/setup");
	await pauseAt(page);
	await page.fill('input[name="token"]', bootstrapToken);
	await page.fill('input[name="display_name"]', "E2E Admin");
	await page.fill('input[name="email"]', "e2e@example.com");
	await page.click('form[data-auth="setup"] button[type="submit"]');
	await page.waitForURL(atPath(base, "/"), { timeout: navTimeout });
	await page.locator('a[href="/admin"]').waitFor({ timeout: uiTimeout });
}

export async function logout(page, base) {
	// Prefer the sidebar's Sign out button; fall back to the endpoint if the
	// session actions are not rendered. The sidebar is a summoned overlay, so
	// open it before reaching the footer button.
	const button = page.locator('[data-auth="logout"]');
	if (await button.count()) {
		if (!(await page.evaluate(() => document.body.dataset.drawer === "sidebar"))) {
			await page.click(".rail-hit");
			await page.waitForSelector('body[data-drawer="sidebar"]', { timeout: uiTimeout });
		}
		await button.click();
	} else {
		const status = await page.evaluate(() => fetch("/api/auth/v1/logout", { method: "POST" }).then((r) => r.status));
		assert.equal(status, 200, "logout");
		await page.goto(base + "/");
	}
	await page.waitForURL(atPath(base, "/login"), { timeout: navTimeout });
}

export async function signIn(page, base) {
	await page.goto(base + "/login");
	await page.click('[data-auth="login"]');
	await page.waitForURL(atPath(base, "/"), { timeout: navTimeout });
}

export async function reportFailure(page, logs) {
	const status = await page?.locator("[data-auth-status]").textContent().catch(() => null);
	if (status) console.error("auth status:", status);
	const alert = await page?.locator("[data-live-alert] .alert-message").textContent().catch(() => null);
	if (alert) console.error("auth alert:", alert.trim());
	console.error("--- app log ---\n" + logs);
}

export async function holdIfAsked() {
	if (headed && holdMs > 0) await sleep(holdMs);
}
