// Browser end-to-end test for the passkey flow. It boots a throwaway instance
// (empty SQLite DB, random port), drives the app's own WebAuthn JS in a real
// Chromium through a CDP virtual authenticator, and asserts setup -> logout ->
// login works. The virtual authenticator is configured like a synced passkey
// (backup eligible), which the Go virtualwebauthn tests do not cover.
import { test } from "node:test";
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { existsSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import net from "node:net";
import { chromium } from "playwright-core";

const repoRoot = dirname(dirname(fileURLToPath(import.meta.url)));
const binary = join(repoRoot, "runbooks");
const bootstrapToken = "e2e-bootstrap-token";

const isTruthy = (v) => v === "1" || v === "true" || v === "yes";
// Headed mode opens a real window so the flow can be watched; slowMo paces the
// Playwright actions so the page transitions are visible. Inspect mode is headed
// plus page.pause() breakpoints, which open the Playwright Inspector; its
// explicit action timeouts are lifted so stepping does not trip them.
const inspect = isTruthy(process.env.E2E_INSPECT);
const headed = isTruthy(process.env.E2E_HEADED) || inspect;
const slowMo = Number(process.env.E2E_SLOWMO || (headed ? 600 : 0));
const navTimeout = inspect ? 0 : 10000;
const uiTimeout = inspect ? 0 : 5000;

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
		await new Promise((r) => setTimeout(r, 150));
	}
	throw new Error(`app not ready after 15s:\n${logs()}`);
}

const atPath = (base, pathname) => (url) => url.origin === new URL(base).origin && url.pathname === pathname;

test("setup, logout and login run the real passkey ceremony", async () => {
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
			IDENTITY_SECURE_COOKIES: "false",
		},
		stdio: ["ignore", "pipe", "pipe"],
	});
	child.stdout.on("data", (chunk) => (logs += chunk));
	child.stderr.on("data", (chunk) => (logs += chunk));

	let browser;
	let page;
	try {
		await waitForReady(base, child, () => logs);

		browser = await chromium.launch({
			executablePath: findChromium(),
			headless: !headed,
			slowMo,
			args: ["--no-sandbox"],
		});
		const context = await browser.newContext();
		page = await context.newPage();
		const cdp = await context.newCDPSession(page);
		// Opens the Playwright Inspector and blocks until Resume/Step in the UI.
		const breakpoint = async () => {
			if (inspect) await page.pause();
		};
		await cdp.send("WebAuthn.enable");
		const { authenticatorId } = await cdp.send("WebAuthn.addVirtualAuthenticator", {
			options: {
				protocol: "ctap2",
				transport: "internal",
				hasResidentKey: true,
				hasUserVerification: true,
				// Chromium names these defaultBackup*, not hasBackup* (which it silently ignores).
				defaultBackupEligibility: true, // synced-passkey shape; the BE flag must round-trip
				defaultBackupState: true,
				isUserVerified: true,
				automaticPresenceSimulation: true,
			},
		});

		// 1. Bootstrap the first admin through the real /setup ceremony.
		await page.goto(base + "/setup");
		await breakpoint();
		await page.fill('input[name="token"]', bootstrapToken);
		await page.fill('input[name="display_name"]', "E2E Admin");
		await page.fill('input[name="email"]', "e2e@example.com");
		await breakpoint();
		await page.click('form[data-auth="setup"] button[type="submit"]');
		await page.waitForURL(atPath(base, "/"), { timeout: navTimeout });

		const { credentials } = await cdp.send("WebAuthn.getCredentials", { authenticatorId });
		assert.equal(credentials.length, 1, "one passkey registered");
		assert.equal(credentials[0].isResidentCredential, true, "passkey is discoverable");
		await page.locator('a[href="/admin"]').waitFor({ timeout: uiTimeout });
		await breakpoint();

		// 2. Log out, then sign back in with the passkey — the path the General
		// BackupEligible regression broke.
		const logoutStatus = await page.evaluate(() => fetch("/api/auth/v1/logout", { method: "POST" }).then((r) => r.status));
		assert.equal(logoutStatus, 200, "logout");
		await page.goto(base + "/");
		await page.waitForURL(atPath(base, "/login"), { timeout: navTimeout });

		await page.click('[data-auth="login"]');
		await page.waitForURL(atPath(base, "/"), { timeout: navTimeout });
		await page.locator('a[href="/admin"]').waitFor({ timeout: uiTimeout });
		await breakpoint();

		if (headed) {
			console.log(`\nHeaded run complete — browser left open at ${base}. Close the window to end the test.\n`);
			const holdMs = Number(process.env.E2E_HOLD_MS || 0);
			if (holdMs > 0) {
				await new Promise((resolve) => setTimeout(resolve, holdMs));
			} else {
				await new Promise((resolve) => browser.on("disconnected", resolve));
			}
		}
	} catch (err) {
		const status = await page?.locator("[data-auth-status]").textContent().catch(() => null);
		if (status) console.error("auth status:", status);
		console.error("--- app log ---\n" + logs);
		throw err;
	} finally {
		if (browser && browser.isConnected()) await browser.close();
		child.kill("SIGTERM");
		await new Promise((r) => setTimeout(r, 500));
		if (child.exitCode === null) child.kill("SIGKILL");
		rmSync(dir, { recursive: true, force: true });
	}
});
