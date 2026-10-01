// Browser end-to-end tests for the identity flow. Each test boots a throwaway
// instance (empty SQLite DB, random port) and drives the app's own WebAuthn JS
// in real Chromium through a CDP virtual authenticator configured like a synced
// passkey (backup eligible) — the shape the Go virtualwebauthn tests do not cover.
//
// Covered: setup -> logout -> login, invite -> member enrolment -> revoke, and
// break-glass recovery. Headed/inspect modes are for watching or stepping.
import { test } from "node:test";
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import net from "node:net";
import { chromium } from "playwright-core";

const repoRoot = dirname(dirname(fileURLToPath(import.meta.url)));
const binary = join(repoRoot, "runbooks");
const bootstrapToken = "e2e-bootstrap-token";
const recoveryToken = "e2e-recovery-token";

const isTruthy = (v) => v === "1" || v === "true" || v === "yes";
const inspect = isTruthy(process.env.E2E_INSPECT);
const headed = isTruthy(process.env.E2E_HEADED) || inspect;
const slowMo = Number(process.env.E2E_SLOWMO || (headed ? 600 : 0));
const holdMs = Number(process.env.E2E_HOLD_MS || 0);
const navTimeout = inspect ? 0 : 10000;
const uiTimeout = inspect ? 0 : 5000;

// snap saves a screenshot when E2E_SCREENSHOT_DIR is set — for eyeballing UI
// changes the assertions cannot judge.
const shotDir = process.env.E2E_SCREENSHOT_DIR;
async function snap(page, name) {
	if (!shotDir) return;
	mkdirSync(shotDir, { recursive: true });
	await page.screenshot({ path: join(shotDir, name + ".png") });
}

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
const atPath = (base, pathname) => (url) => url.origin === new URL(base).origin && url.pathname === pathname;

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

async function waitForReady(base, child, logs) {
	const deadline = Date.now() + 15000;
	while (Date.now() < deadline) {
		if (child.exitCode !== null) throw new Error(`app exited early:\n${logs()}`);
		try {
			const res = await fetch(base + "/setup");
			if (res.ok) return;
		} catch {
			// not up yet
		}
		await sleep(150);
	}
	throw new Error(`app not ready after 15s:\n${logs()}`);
}

// startApp boots the built binary with a throwaway database on a free port.
async function startApp() {
	const port = await freePort();
	const base = `http://localhost:${port}`;
	const dir = mkdtempSync(join(tmpdir(), "runbooks-e2e-"));
	let logs = "";
	const child = spawn(binary, [], {
		cwd: repoRoot,
		env: {
			...process.env,
			PORT: String(port),
			IDENTITY_DB_DRIVER: "sqlite",
			IDENTITY_DB_DSN: `file:${join(dir, "identity.db")}`,
			IDENTITY_PUBLIC_URL: base,
			IDENTITY_BOOTSTRAP_TOKEN: bootstrapToken,
			IDENTITY_RECOVERY_TOKEN: recoveryToken,
			IDENTITY_SECURE_COOKIES: "false",
		},
		stdio: ["ignore", "pipe", "pipe"],
	});
	child.stdout.on("data", (chunk) => (logs += chunk));
	child.stderr.on("data", (chunk) => (logs += chunk));
	try {
		await waitForReady(base, child, () => logs);
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
async function startBrowser() {
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
const pauseAt = (page) => (inspect ? page.pause() : Promise.resolve());

async function bootAdmin(page, base) {
	await page.goto(base + "/setup");
	await pauseAt(page);
	await page.fill('input[name="token"]', bootstrapToken);
	await page.fill('input[name="display_name"]', "E2E Admin");
	await page.fill('input[name="email"]', "e2e@example.com");
	await page.click('form[data-auth="setup"] button[type="submit"]');
	await page.waitForURL(atPath(base, "/"), { timeout: navTimeout });
	await page.locator('a[href="/admin"]').waitFor({ timeout: uiTimeout });
}

async function logout(page, base) {
	// Prefer the sidebar's Sign out button; fall back to the endpoint if the
	// session actions are not rendered.
	const button = page.locator('[data-auth="logout"]');
	if (await button.count()) {
		await button.click();
	} else {
		const status = await page.evaluate(() => fetch("/api/auth/v1/logout", { method: "POST" }).then((r) => r.status));
		assert.equal(status, 200, "logout");
		await page.goto(base + "/");
	}
	await page.waitForURL(atPath(base, "/login"), { timeout: navTimeout });
}

async function signIn(page, base) {
	await page.goto(base + "/login");
	await page.click('[data-auth="login"]');
	await page.waitForURL(atPath(base, "/"), { timeout: navTimeout });
}

async function reportFailure(page, logs) {
	const status = await page?.locator("[data-auth-status]").textContent().catch(() => null);
	if (status) console.error("auth status:", status);
	console.error("--- app log ---\n" + logs);
}

async function holdIfAsked() {
	if (headed && holdMs > 0) await sleep(holdMs);
}

test("setup, logout and login run the real passkey ceremony", async (t) => {
	const app = await startApp();
	t.after(() => app.stop());
	const b = await startBrowser();
	t.after(() => b.stop());
	try {
		await bootAdmin(b.page, app.base);
		await snap(b.page, "index");
		const { credentials } = await b.cdp.send("WebAuthn.getCredentials", { authenticatorId: b.authenticatorId });
		assert.equal(credentials.length, 1, "one passkey registered");
		assert.equal(credentials[0].isResidentCredential, true, "passkey is discoverable");
		await pauseAt(b.page);

		await logout(b.page, app.base);
		await signIn(b.page, app.base);
		await b.page.locator('a[href="/admin"]').waitFor({ timeout: uiTimeout });

		// The design-system styleguide (inside the app shell), and an anchored
		// section so the scroll offset is visible.
		await b.page.goto(app.base + "/styleguide");
		await snap(b.page, "styleguide");
		await b.page.goto(app.base + "/styleguide#step");
		await snap(b.page, "styleguide-anchor");
		await b.page.goto(app.base + "/styleguide#field");
		await snap(b.page, "styleguide-field");
		await b.page.goto(app.base + "/styleguide#layouts");
		await snap(b.page, "styleguide-layouts");

		// The kitchen-sink runbook, for eyeballing every block type.
		await b.page.goto(app.base + "/gallery");
		await b.page.fill('.var-input[data-var="HOST"]', "db-2.prod.internal");
		await snap(b.page, "runbook");

		// The destructive Clear confirmation is a real dialog, not window.confirm().
		await b.page.locator(".notes-clear").click();
		await b.page.locator("dialog.dialog").waitFor({ timeout: uiTimeout });
		await snap(b.page, "dialog");
		await b.page.locator('dialog.dialog .btn-ghost').click();
		await b.page.evaluate(() => localStorage.setItem("runbooks-theme", "light"));
		await b.page.reload();
		await b.page.locator(".code-group").first().scrollIntoViewIfNeeded();
		await snap(b.page, "runbook-light-code");
		await b.page.locator(".rollback-card").scrollIntoViewIfNeeded();
		await snap(b.page, "runbook-light-rollback");
		if (process.env.E2E_DEBUG) {
			const info = await b.page.evaluate(() => {
				const cs = (sel) => {
					const el = document.querySelector(sel);
					if (!el) return "MISSING";
					const s = getComputedStyle(el);
					return `${s.fontFamily} | w${s.fontWeight} | ${s.fontSize}`;
				};
				const bg = (sel) => {
					const el = document.querySelector(sel);
					return el ? getComputedStyle(el).backgroundColor : "MISSING";
				};
				return {
					body: cs("body"),
					adminLink: cs(".nav-admin a"),
					navLink: cs(".sidebar nav a"),
					appearanceLabel: cs(".appearance-label"),
					codeBlockBg: bg(".code-block"),
					codeBg: bg(".code-group .code-block"),
				};
			});
			console.log("E2E font/colour probe:", JSON.stringify(info, null, 2));
		}
		await holdIfAsked();
	} catch (err) {
		await reportFailure(b.page, app.logs());
		throw err;
	}
});

test("an admin invites a member, who enrols, and the admin revokes their sessions", async (t) => {
	const app = await startApp();
	t.after(() => app.stop());
	const admin = await startBrowser();
	t.after(() => admin.stop());
	const member = await startBrowser();
	t.after(() => member.stop());
	try {
		await bootAdmin(admin.page, app.base);

		// The admin mints an invite from the /admin page.
		await admin.page.goto(app.base + "/admin");
		await snap(admin.page, "admin");
		await admin.page.selectOption('form[data-admin="invite"] select[name="role"]', "member");
		await admin.page.click('form[data-admin="invite"] button[type="submit"]');
		await admin.page.locator("[data-admin-result]").waitFor({ state: "visible", timeout: uiTimeout });
		const inviteUrl = await admin.page.inputValue("[data-admin-url]");
		assert.match(inviteUrl, /\/invite\//);

		// The invited member enrols in their own browser (own cookie jar).
		await member.page.goto(inviteUrl);
		await pauseAt(member.page);
		await member.page.fill('input[name="display_name"]', "E2E Member");
		await member.page.fill('input[name="email"]', "member@example.com");
		await member.page.click('form[data-auth="invite"] button[type="submit"]');
		await member.page.waitForURL(atPath(app.base, "/"), { timeout: navTimeout });
		assert.equal(await member.page.locator('a[href="/admin"]').count(), 0, "member is not an admin");

		// The admin revokes the member's sessions from the users table.
		await admin.page.reload();
		await admin.page.locator("tr", { hasText: "E2E Member" }).locator('[data-admin="revoke"]').click();
		await admin.page.locator("[data-admin-status]").filter({ hasText: "Sessions revoked" }).waitFor({ timeout: uiTimeout });

		// The member is bounced on their next request.
		await member.page.goto(app.base + "/");
		await member.page.waitForURL(atPath(app.base, "/login"), { timeout: navTimeout });
		await holdIfAsked();
	} catch (err) {
		await reportFailure(admin.page, app.logs());
		throw err;
	}
});

test("break-glass recovery re-enrols the admin's passkey", async (t) => {
	const app = await startApp();
	t.after(() => app.stop());
	const b = await startBrowser();
	t.after(() => b.stop());
	try {
		await bootAdmin(b.page, app.base);

		// Lose the passkey: drop every credential from the authenticator.
		await b.cdp.send("WebAuthn.clearCredentials", { authenticatorId: b.authenticatorId });
		await logout(b.page, app.base);
		await b.page.goto(app.base + "/login");
		await b.page.click('[data-auth="login"]');
		await b.page.locator("[data-auth-status]").filter({ hasText: "Sign in failed" }).waitFor({ timeout: navTimeout });

		// Break-glass re-enrols a passkey for the sole admin.
		await b.page.goto(app.base + "/recovery");
		await pauseAt(b.page);
		await b.page.fill('input[name="token"]', recoveryToken);
		await b.page.click('form[data-auth="recovery"] button[type="submit"]');
		await b.page.waitForURL(atPath(app.base, "/"), { timeout: navTimeout });
		await b.page.locator('a[href="/admin"]').waitFor({ timeout: uiTimeout });

		// The re-enrolled passkey signs in on its own.
		await logout(b.page, app.base);
		await signIn(b.page, app.base);
		await b.page.locator('a[href="/admin"]').waitFor({ timeout: uiTimeout });
		await holdIfAsked();
	} catch (err) {
		await reportFailure(b.page, app.logs());
		throw err;
	}
});

test("break-glass recovers an abandoned /setup (admin user with no credential)", async (t) => {
	const app = await startApp();
	t.after(() => app.stop());
	const b = await startBrowser();
	t.after(() => b.stop());
	try {
		// Abandon the first-admin ceremony: setup/begin creates the admin user and
		// we never finish, so the instance is locked with no credential at all.
		const begin = await b.page.request.post(app.base + "/api/auth/v1/setup/begin", {
			data: { token: bootstrapToken, display_name: "Locked Admin", email: "locked@example.com" },
		});
		assert.equal(begin.status(), 200, "setup/begin creates the admin user");

		// /setup is now closed, and there is no passkey to sign in with.
		await b.page.goto(app.base + "/setup");
		await b.page.waitForURL(atPath(app.base, "/login"), { timeout: navTimeout });
		await b.page.click('[data-auth="login"]');
		await b.page.locator("[data-auth-status]").filter({ hasText: "Sign in failed" }).waitFor({ timeout: navTimeout });

		// Break-glass re-enrols a passkey for the orphaned admin and gets back in.
		await b.page.goto(app.base + "/recovery");
		await pauseAt(b.page);
		await b.page.fill('input[name="token"]', recoveryToken);
		await b.page.click('form[data-auth="recovery"] button[type="submit"]');
		await b.page.waitForURL(atPath(app.base, "/"), { timeout: navTimeout });
		await b.page.locator('a[href="/admin"]').waitFor({ timeout: uiTimeout });
		await holdIfAsked();
	} catch (err) {
		await reportFailure(b.page, app.logs());
		throw err;
	}
});
