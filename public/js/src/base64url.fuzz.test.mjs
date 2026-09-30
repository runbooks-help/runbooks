// Differential fuzzing for base64url.mjs against Node's own base64url codec.
// Seeded PRNG so any failure is reproducible; FUZZ_ITERATIONS raises the budget
// for a longer round.
import assert from 'node:assert/strict';
import { test } from 'node:test';

import { decodeBase64URL, encodeBase64URL } from './base64url.mjs';

const ITERATIONS = Number(process.env.FUZZ_ITERATIONS || 5000);

// mulberry32: tiny seeded PRNG.
function prng(seed) {
	return function next() {
		seed = (seed + 0x6d2b79f5) | 0;
		let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
		t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
		return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
	};
}

function fill(rand, n) {
	const bytes = new Uint8Array(n);
	for (let i = 0; i < n; i++) bytes[i] = Math.floor(rand() * 256);
	return bytes;
}

test('fuzz: encode/decode match Node and round-trip', () => {
	const rand = prng(0xc0ffee);
	for (let i = 0; i < ITERATIONS; i++) {
		const bytes = fill(rand, Math.floor(rand() * 96));
		const encoded = encodeBase64URL(bytes);
		const reference = Buffer.from(bytes).toString('base64url');
		assert.equal(encoded, reference, `encode mismatch for ${bytes.length} bytes`);
		assert.ok(!/[+/=]/.test(encoded), `not URL-safe: ${encoded}`);
		assert.deepEqual(decodeBase64URL(encoded), bytes, `round trip for ${bytes.length} bytes`);
		assert.deepEqual(decodeBase64URL(reference), bytes, `reference decode for ${bytes.length} bytes`);
	}
});

test('fuzz: typed-array views with random offsets', () => {
	const rand = prng(0xbeef);
	for (let i = 0; i < ITERATIONS; i++) {
		const buffer = fill(rand, Math.floor(rand() * 64) + 1);
		const offset = Math.floor(rand() * buffer.length);
		const view = new Uint8Array(buffer.buffer, offset, buffer.length - offset);
		assert.equal(
			encodeBase64URL(view),
			Buffer.from(view).toString('base64url'),
			`view offset=${offset} len=${view.length}`,
		);
	}
});

test('fuzz: ArrayBuffer input equals the same bytes', () => {
	const rand = prng(0xfeed);
	for (let i = 0; i < ITERATIONS; i++) {
		const bytes = fill(rand, Math.floor(rand() * 32));
		assert.equal(encodeBase64URL(bytes.buffer), Buffer.from(bytes).toString('base64url'));
	}
});

test('fuzz: arbitrary strings only ever decode or throw', () => {
	const rand = prng(0x1234);
	for (let i = 0; i < ITERATIONS; i++) {
		let s = '';
		const n = Math.floor(rand() * 24);
		for (let j = 0; j < n; j++) s += String.fromCharCode(Math.floor(rand() * 256));
		let out;
		try {
			out = decodeBase64URL(s);
		} catch (err) {
			assert.ok(err instanceof Error, `threw a non-Error for ${JSON.stringify(s)}`);
			continue;
		}
		assert.ok(out instanceof Uint8Array, `decode returned ${typeof out} for ${JSON.stringify(s)}`);
		// Whatever it returned must survive a re-encode/re-decode.
		assert.deepEqual(decodeBase64URL(encodeBase64URL(out)), out, `unstable for ${JSON.stringify(s)}`);
	}
});
