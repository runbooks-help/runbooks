// SPDX-License-Identifier: FSL-1.1-MIT

// WebAuthn options and responses carry binary fields as base64url strings.
// These convert between those strings and byte arrays. atob/btoa are the only
// browser primitives, so the padding and URL-safe handling lives here, in one
// place, and is unit-tested (base64url.test.mjs).

export function decodeBase64URL(value) {
	const s = value.replace(/-/g, '+').replace(/_/g, '/');
	const padded = s + '='.repeat((4 - (s.length % 4)) % 4);
	const bin = atob(padded);
	const bytes = new Uint8Array(bin.length);
	for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
	return bytes;
}

export function encodeBase64URL(value) {
	if (value == null) return undefined;
	const bytes =
		value instanceof ArrayBuffer
			? new Uint8Array(value)
			: new Uint8Array(value.buffer, value.byteOffset, value.byteLength);
	let bin = '';
	for (let i = 0; i < bytes.length; i++) bin += String.fromCharCode(bytes[i]);
	return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
}
