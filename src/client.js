import os from 'node:os';
import process from 'node:process';
import WebSocket from 'ws';

const DEFAULT_SOCKET_URL = 'ws://localhost:3333/api/v1/live';

export function resolveSocketUrl(value = process.env.DIDBAN_WS_URL ?? DEFAULT_SOCKET_URL) {
  const url = new URL(value);
  if (url.protocol === 'http:') url.protocol = 'ws:';
  if (url.protocol === 'https:') url.protocol = 'wss:';
  if (url.protocol !== 'ws:' && url.protocol !== 'wss:') {
    throw new TypeError('DIDBAN_WS_URL must use ws, wss, http or https');
  }
  if (url.pathname === '/' || url.pathname === '') url.pathname = '/api/v1/live';
  if (url.pathname.replace(/\/$/, '') === '/api') url.pathname = '/api/v1/live';
  url.search = '';
  url.hash = '';
  return url.toString();
}

function required(value, name) {
  if (typeof value !== 'string' || !value.trim()) throw new TypeError(`${name} is required`);
  return value.trim();
}

function cpuTimes() {
  const cores = os.cpus().map((cpu) => ({
    idle: cpu.times.idle,
    all: Object.values(cpu.times).reduce((sum, value) => sum + value, 0),
  }));
  return {
    cores,
    idle: cores.reduce((sum, core) => sum + core.idle, 0),
    all: cores.reduce((sum, core) => sum + core.all, 0),
  };
}

export class DidbanServerAgent {
  #socket;
  #telemetryTimer;
  #reconnectTimer;
  #stopped = true;
  #attempt = 0;
  #lastCpu = cpuTimes();
  #queue = [];
  #processHandlers;

  constructor(options = {}) {
    this.serverUrl = resolveSocketUrl(options.serverUrl);
    this.apiKey = required(options.apiKey ?? process.env.DIDBAN_API_KEY, 'DIDBAN_API_KEY');
    this.agentId = required(
      options.agentId ?? process.env.DIDBAN_AGENT_ID ?? `${os.hostname()}-${os.platform()}-${os.arch()}`,
      'DIDBAN_AGENT_ID',
    );
    this.name = required(options.name ?? process.env.DIDBAN_AGENT_NAME ?? os.hostname(), 'DIDBAN_AGENT_NAME');
    this.version = options.version ?? '0.1.0';
    this.intervalMs = Math.max(
      1_000,
      Number(options.intervalMs ?? process.env.DIDBAN_AGENT_INTERVAL_MS ?? 5_000),
    );
    this.onStatus = options.onStatus;
    this.WebSocketImpl = options.WebSocketImpl ?? WebSocket;
    this.captureProcessErrors =
      options.captureProcessErrors ?? process.env.DIDBAN_CAPTURE_PROCESS_ERRORS !== 'false';
  }

  start() {
    if (!this.#stopped) return this;
    this.#stopped = false;
    if (this.captureProcessErrors) this.#attachProcessHandlers();
    this.#connect();
    return this;
  }

  stop() {
    this.#stopped = true;
    clearTimeout(this.#reconnectTimer);
    clearInterval(this.#telemetryTimer);
    this.#socket?.close(1000, 'Agent stopped');
    this.#socket = undefined;
    this.#detachProcessHandlers();
    this.onStatus?.({ state: 'stopped' });
  }

  log(message, data, level = 'info') {
    const payload = {
      type: 'log',
      timestamp: new Date().toISOString(),
      level,
      message: String(message).slice(0, 500),
      ...(data && typeof data === 'object' ? { data } : {}),
    };
    if (!this.#send(payload)) {
      this.#queue.push(payload);
      if (this.#queue.length > 100) this.#queue.shift();
    }
  }

  captureException(error, data) {
    const normalized = error instanceof Error ? error : new Error(String(error));
    this.log(normalized.message, { name: normalized.name, stack: normalized.stack, ...data }, 'error');
  }

  #attachProcessHandlers() {
    if (this.#processHandlers) return;
    const uncaught = (error, origin) => this.captureException(error, { origin, source: 'process' });
    const warning = (value) =>
      this.log(value.message, { name: value.name, stack: value.stack, source: 'process.warning' }, 'warning');
    process.on('uncaughtExceptionMonitor', uncaught);
    process.on('warning', warning);
    this.#processHandlers = { uncaught, warning };
  }

  #detachProcessHandlers() {
    if (!this.#processHandlers) return;
    process.off('uncaughtExceptionMonitor', this.#processHandlers.uncaught);
    process.off('warning', this.#processHandlers.warning);
    this.#processHandlers = undefined;
  }

  #connect() {
    if (this.#stopped) return;
    this.onStatus?.({ state: 'connecting', serverUrl: this.serverUrl });
    const socket = new this.WebSocketImpl(this.serverUrl);
    this.#socket = socket;
    socket.addEventListener('open', () => {
      this.#send({
        type: 'auth',
        role: 'agent',
        apiKey: this.apiKey,
        agent: {
          id: this.agentId,
          name: this.name,
          version: this.version,
          hostname: os.hostname(),
          platform: `${os.platform()} ${os.release()} ${os.arch()}`,
        },
      });
    });
    socket.addEventListener('message', (message) => {
      try {
        const payload = JSON.parse(String(message.data));
        if (payload.type !== 'auth.ok') return;
        this.#attempt = 0;
        this.onStatus?.({ state: 'connected', projectId: payload.projectId });
        clearInterval(this.#telemetryTimer);
        this.#telemetryTimer = setInterval(() => this.#report(), this.intervalMs);
        this.#report();
        for (const queued of this.#queue.splice(0)) this.#send(queued);
      } catch (error) {
        this.onStatus?.({ state: 'error', error });
      }
    });
    socket.addEventListener('close', (event) => {
      clearInterval(this.#telemetryTimer);
      if (this.#socket === socket) this.#socket = undefined;
      if (this.#stopped) return;
      this.#attempt += 1;
      const delay = Math.min(30_000, 1_000 * 2 ** Math.min(this.#attempt, 5));
      this.onStatus?.({ state: 'disconnected', code: event.code, retryInMs: delay });
      this.#reconnectTimer = setTimeout(() => this.#connect(), delay);
    });
    socket.addEventListener('error', (error) => this.onStatus?.({ state: 'error', error }));
  }

  #report() {
    const current = cpuTimes();
    const totalDelta = current.all - this.#lastCpu.all;
    const idleDelta = current.idle - this.#lastCpu.idle;
    const cpuCoresPercent = current.cores.map((core, index) => {
      const previous = this.#lastCpu.cores[index] ?? core;
      const coreTotalDelta = core.all - previous.all;
      const coreIdleDelta = core.idle - previous.idle;
      const percent = coreTotalDelta > 0 ? ((coreTotalDelta - coreIdleDelta) / coreTotalDelta) * 100 : 0;
      return Math.round(percent * 10) / 10;
    });
    this.#lastCpu = current;
    const cpuPercent = totalDelta > 0 ? ((totalDelta - idleDelta) / totalDelta) * 100 : 0;
    this.#send({
      type: 'telemetry',
      timestamp: new Date().toISOString(),
      level: 'info',
      metrics: {
        cpuPercent: Math.round(cpuPercent * 10) / 10,
        cpuCoresPercent,
        memoryUsedBytes: os.totalmem() - os.freemem(),
        memoryTotalBytes: os.totalmem(),
        processUptimeSeconds: process.uptime(),
        systemUptimeSeconds: os.uptime(),
        loadAverage1: os.loadavg()[0],
      },
    });
  }

  #send(payload) {
    if (!this.#socket || this.#socket.readyState !== this.WebSocketImpl.OPEN) return false;
    this.#socket.send(JSON.stringify(payload));
    return true;
  }
}
