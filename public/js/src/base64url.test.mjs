import assert from 'node:assert/strict';
import { test } from 'node:test';

import { decodeBase64URL, encodeBase64URL } from './base64url.mjs';

const bytes = (...b) => new Uint8Array(b);

test('encode: known vectors', () => {
	assert.equal(encodeBase64URL(bytes()), '');
	assert.equal(encodeBase64URL(bytes(0x00)), 'AA');
	assert.equal(encodeBase64URL(bytes(0xff)), '_w');
	assert.equal(encodeBase64URL(bytes(0xfb, 0xff)), '-_8');
	assert.equal(encodeBase64URL(bytes(1, 2, 3)), 'AQID');
});

test('encode: URL-safe alphabet, no padding', () => {
	// "+/8=" in standard base64 becomes "-_8" here.
	const encoded = encodeBase64URL(bytes(0xfb, 0xff));
	assert.ok(encoded.includes('-'), 'plus is mapped to -');
	assert.ok(encoded.includes('_'), 'slash is mapped to _');
	assert.ok(!encoded.includes('+') && !encoded.includes('/') && !encoded.includes('='));
});

test('encode: accepts ArrayBuffer and typed-array views', () => {
	const ab = new ArrayBuffer(3);
	new Uint8Array(ab).set([1, 2, 3]);
	assert.equal(encodeBase64URL(ab), 'AQID');
	const view = new Uint8Array(ab, 1, 2); // bytes 2,3
	assert.equal(encodeBase64URL(view), 'AgM');
});

test('encode: null/undefined stays undefined (userHandle can be absent)', () => {
	assert.equal(encodeBase64URL(null), undefined);
	assert.equal(encodeBase64URL(undefined), undefined);
});

test('decode: known vectors, padded and unpadded', () => {
	assert.deepEqual(decodeBase64URL(''), bytes());
	assert.deepEqual(decodeBase64URL('AA'), bytes(0x00));
	assert.deepEqual(decodeBase64URL('AQID'), bytes(1, 2, 3));
	assert.deepEqual(decodeBase64URL('AQI='), bytes(1, 2));
});

test('round trip for every length mod 4', () => {
	for (let n = 0; n <= 12; n++) {
		const src = Uint8Array.from({ length: n }, (_, i) => (i * 37 + 11) % 256);
		assert.deepEqual(decodeBase64URL(encodeBase64URL(src)), src, `length ${n}`);
	}
});

test('decode: an impossible base64 length throws', () => {
	assert.throws(() => decodeBase64URL('A'));
});
