#!/usr/bin/env node
import { DidbanServerAgent } from '../src/index.js';

const agent = new DidbanServerAgent({
  onStatus(status) {
    const detail = status.serverUrl ?? status.projectId ?? status.retryInMs ?? '';
    console.log(`[didban-agent] ${status.state}`, detail);
  },
});

agent.start();
for (const signal of ['SIGINT', 'SIGTERM']) {
  process.once(signal, () => {
    agent.stop();
    process.exit(0);
  });
}
