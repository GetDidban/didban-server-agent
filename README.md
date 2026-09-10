# Didban Server Agent

Agent سبک دیدبان برای ارسال زندهٔ مصرف CPU، تک‌تک هسته‌ها، حافظه، uptime و رخدادهای عملیاتی سرور به «نبض سرورها».

## نصب سریع روی Linux

از صفحهٔ «نبض سرورها» در دیدبان، دستور مخصوص پروژه را کپی و روی سرور اجرا کنید. دستور شبیه نمونهٔ زیر است:

```bash
curl -fsSL https://raw.githubusercontent.com/GetDidban/didban-server-agent/master/install.sh | sudo bash -s -- --api-key 'didban_...' --server 'wss://api.getdidban.ir/api/v1/live' --name 'Production API'
```

این نصب‌کننده:

- باینری مستقل مناسب `amd64` یا `arm64` را از آخرین GitHub Release دریافت می‌کند و تا پیش از اولین Release از build آمادهٔ مخزن استفاده می‌کند؛
- checksum فایل را پیش از نصب بررسی می‌کند؛
- تنظیمات را با دسترسی محدود در `/etc/didban-agent.env` ذخیره می‌کند؛
- سرویس `didban-agent.service` را با systemd ثبت، فعال و اجرا می‌کند؛
- به Node.js، npm، Docker، jq یا websocat نیاز ندارد.

```bash
# وضعیت سرویس
sudo systemctl status didban-agent --no-pager

# مشاهده لاگ زنده
sudo journalctl -u didban-agent -f
```

برای نصب مجدد یا تغییر تنظیمات، همان دستور نصب را با مقادیر جدید دوباره اجرا کنید.

آدرس پیش‌فرض دیدبان `wss://api.getdidban.ir/api/v1/live` است. متغیرهای قابل تنظیم:

| متغیر | کاربرد | مقدار پیش‌فرض |
| --- | --- | --- |
| `DIDBAN_API_KEY` | توکن پروژه؛ الزامی | — |
| `DIDBAN_WS_URL` | آدرس WebSocket دیدبان | `wss://api.getdidban.ir/api/v1/live` |
| `DIDBAN_AGENT_NAME` | نام نمایشی سرور | hostname |
| `DIDBAN_AGENT_ID` | شناسه پایدار Agent | hostname + platform |
| `DIDBAN_AGENT_INTERVAL_MS` | فاصله ارسال متریک | `5000` |

> دستور نصب حاوی توکن پروژه است. آن را در تیکت، چت عمومی یا history مشترک قرار ندهید و در صورت افشا، توکن پروژه را عوض کنید.

## انتشار باینری

با push کردن tag مثل `v0.2.0`، workflow انتشار باینری‌های Linux برای `amd64` و `arm64` را می‌سازد، تست می‌کند، checksum تولید می‌کند و فایل‌ها را به GitHub Release همان tag اضافه می‌کند.

برای build محلی:

```bash
go test ./...
CGO_ENABLED=0 go build -trimpath -o didban-agent ./cmd/didban-agent
```

## استفاده به‌عنوان SDK در Node.js

نسخهٔ JavaScript برای اتصال مستقیم یک برنامهٔ Node.js همچنان موجود است:

```bash
npm install
```

```js
import { DidbanServerAgent } from 'didban-server-agent';

const agent = new DidbanServerAgent({
  serverUrl: 'wss://api.getdidban.ir/api/v1/live',
  apiKey: process.env.DIDBAN_API_KEY,
  agentId: 'checkout-api-1',
  name: 'Checkout API',
});

agent.start();
agent.log('worker started', { queue: 'payments' });
agent.captureException(new Error('database timeout'), { region: 'ir-1' });
```
