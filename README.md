# Didban Server Agent

Agent سبک Node.js برای ارسال زنده‌ی مصرف کلی و تک‌تک هسته‌های CPU، حافظه، uptime و رخدادهای سفارشی به Central Didban با WebSocket.

## اجرا

```bash
npm install

# PowerShell
$env:DIDBAN_WS_URL="ws://192.168.3.104:3333/api/v1/live"
$env:DIDBAN_API_KEY="didban_..."
$env:DIDBAN_AGENT_ID="server-1"
$env:DIDBAN_AGENT_NAME="Production API"
npm start
```

`DIDBAN_WS_URL` می‌تواند origin ساده مثل `http://192.168.3.104:3333` نیز باشد؛ Agent آن را به آدرس WebSocket تبدیل می‌کند. برای HTTPS از `wss://` استفاده کنید.

## استفاده به‌عنوان SDK

```js
import { DidbanServerAgent } from 'didban-server-agent';

const agent = new DidbanServerAgent({
  serverUrl: process.env.MONITORING_URL,
  apiKey: process.env.MONITORING_API_KEY,
  agentId: 'checkout-api-1',
  name: 'Checkout API',
  intervalMs: 5_000,
  captureProcessErrors: true,
});

agent.start();
agent.log('worker started', { queue: 'payments' });
agent.captureException(new Error('database timeout'), { region: 'ir-1' });
```

Agent به‌صورت پیش‌فرض خطاهای `uncaughtExceptionMonitor` و هشدارهای Process را نیز گزارش می‌کند؛ این رفتار را با `DIDBAN_CAPTURE_PROCESS_ERRORS=false` یا گزینه‌ی constructor خاموش کنید. برای خطاهای مدیریت‌شده از `captureException` و برای لاگ‌های برنامه از `log(message, data, level)` استفاده کنید.

اولویت تنظیمات با constructor است و سپس متغیرهای محیطی خوانده می‌شوند. در نتیجه آدرس سرور بدون تغییر کد قابل عوض‌شدن است.
