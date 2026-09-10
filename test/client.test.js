import test from 'node:test';
import assert from 'node:assert/strict';
import { resolveSocketUrl } from '../src/index.js';

test('uses an explicit complete WebSocket endpoint', () => {
  assert.equal(resolveSocketUrl('wss://didban.example/live/socket'), 'wss://didban.example/live/socket');
});

test('converts an HTTP server origin into the live endpoint', () => {
  assert.equal(resolveSocketUrl('http://10.0.0.8:3333'), 'ws://10.0.0.8:3333/api/v1/live');
  assert.equal(resolveSocketUrl('https://didban.example/api'), 'wss://didban.example/api/v1/live');
});
