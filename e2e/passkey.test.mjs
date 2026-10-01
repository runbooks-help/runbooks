// Identity E2E: setup -> logout -> login, invite -> member enrol -> revoke, and
// break-glass recovery — driven through the app's real WebAuthn JS in Chromium
// via a virtual authenticator. Runbook-page behaviour lives in runbook.test.mjs
// and the design system in styleguide.test.mjs.
import { test } from "node:test";
import assert from "node:assert/strict";
import {
	bootstrapToken,
	recoveryToken,
	atPath,
	navTimeout,
	uiTimeout,
	startApp,
	startBrowser,
	bootAdmin,
	logout,
	signIn,
	pauseAt,
	snap,
	reportFailure,
	holdIfAsked,
} from "./harness.mjs";

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
		await b.page.locator("[data-live-alert] .alert-title").filter({ hasText: "Sign in failed" }).waitFor({ timeout: navTimeout });

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

test("break-glass recovers an orphaned admin (abandoned /setup)", async (t) => {
	const app = await startApp();
	t.after(() => app.stop());
	const b = await startBrowser();
	t.after(() => b.stop());
	try {
		// Abandon the first-admin ceremony: setup/begin creates the admin user and
		// we never finish, leaving an admin row with no credential.
		const begin = await b.page.request.post(app.base + "/api/auth/v1/setup/begin", {
			data: { token: bootstrapToken, display_name: "Locked Admin", email: "locked@example.com" },
		});
		assert.equal(begin.status(), 200, "setup/begin creates the admin user");

		// An admin with no credential does not close bootstrap: /setup reopens so the
		// operator can retry the ceremony.
		await b.page.goto(app.base + "/setup");
		await b.page.locator('form[data-auth="setup"]').waitFor({ timeout: uiTimeout });

		// Break-glass still re-enrols a passkey for the orphaned admin and gets back in.
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
