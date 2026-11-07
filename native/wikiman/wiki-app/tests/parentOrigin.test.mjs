import assert from 'node:assert/strict';
import {test} from 'node:test';
import {validEditorParentOrigin} from '../src/portable/parentOrigin.ts';

test('accepts a remote Web parent only at the editor document origin', () => {
    const previous = Object.getOwnPropertyDescriptor(globalThis, 'location');
    Object.defineProperty(globalThis, 'location', {configurable: true, value: new URL('https://agentwork.internal')});
    try {
        assert.equal(validEditorParentOrigin('https://agentwork.internal'), true);
        assert.equal(validEditorParentOrigin('https://other.internal'), false);
        assert.equal(validEditorParentOrigin('http://agentwork.internal'), false);
    } finally {
        if (previous) {Object.defineProperty(globalThis, 'location', previous);}
        else {delete globalThis.location;}
    }
});

for (const origin of ['http://127.0.0.1:49615', 'http://localhost:9000', 'http://[::1]:4000', 'tauri://localhost', 'dsh-app://app']) {
    test('accepts the canonical local application origin ' + origin, () => {
        assert.equal(validEditorParentOrigin(origin), true);
    });
}
for (const origin of ['*', 'null', '', 'dsh-app://shell', 'dsh-app://attacker', 'dsh-app://app/', 'dsh-app://app:80',
    'dsh-app://user@app', 'dsh-app://app?token=x', 'tauri://attacker', 'http://attacker.invalid',
    'http://127.0.0.1:9000/path', 'http://user@127.0.0.1:9000', 'http://127.0.0.1:9000#fragment']) {
    test('rejects the untrusted or noncanonical origin ' + origin, () => {
        assert.equal(validEditorParentOrigin(origin), false);
    });
}
