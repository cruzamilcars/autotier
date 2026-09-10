import assert from 'node:assert/strict';
import { test } from 'node:test';
import { apply } from '../lib/index.js';
import { callArgs, routeArgs, statusArgs } from '../lib/runner.js';

function fakeCtx() {
  const defs = new Map();
  return {
    defs,
    tools: {
      register(def) {
        defs.set(def.name, def);
      },
    },
  };
}

const exec = { signal: AbortController ? new AbortController().signal : undefined };

test('registers 3 tools', () => {
  const ctx = fakeCtx();
  apply(ctx, {});
  assert.deepEqual(
    [...ctx.defs.keys()].sort(),
    ['autotier_call', 'autotier_route', 'autotier_status'],
  );
});

test('argv builders put flags before positionals', () => {
  assert.deepEqual(routeArgs('hi', { mode: 'cost' }), ['route', '--mode', 'cost', 'hi']);
  assert.deepEqual(callArgs('explore', 't', { to: 'judge' }), [
    'agent',
    'call',
    '--to',
    'judge',
    'explore',
    't',
  ]);
  assert.deepEqual(statusArgs(), ['status']);
});

test('autotier_route dry-run decides without spending', async () => {
  const ctx = fakeCtx();
  apply(ctx, {});
  const out = await ctx.defs.get('autotier_route').execute(
    { prompt: 'busca donde se define login', mode: 'cost' },
    exec,
  );
  assert.match(out, /decision=haiku-tier/);
  assert.match(out, /\$0 gastados/);
});

test('autotier_call runs a named agent', async () => {
  const ctx = fakeCtx();
  apply(ctx, {});
  const out = await ctx.defs.get('autotier_call').execute(
    { agent: 'explore', task: 'lista archivos' },
    exec,
  );
  assert.match(out, /@explore/);
});

test('autotier_status reports savings', async () => {
  const ctx = fakeCtx();
  apply(ctx, {});
  const out = await ctx.defs.get('autotier_status').execute({}, exec);
  assert.match(out, /requests:|sin datos|ahorro/);
});
