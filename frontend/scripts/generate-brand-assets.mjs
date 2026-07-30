import { readFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright";

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..");
const publicDir = join(frontendRoot, "public");
const logo = await readFile(join(publicDir, "favicon.svg"), "utf8");
const logoData = `data:image/svg+xml;base64,${Buffer.from(logo).toString("base64")}`;
const browser = await chromium.launch();

async function renderIcon(size, output) {
  const page = await browser.newPage({ viewport: { width: size, height: size } });
  await page.setContent(`
    <style>
      * { box-sizing: border-box; }
      html, body { margin: 0; width: 100%; height: 100%; background: #09090b; }
      body { display: grid; place-items: center; }
      img { width: 82%; height: 82%; }
    </style>
    <img src="${logoData}" alt="" />
  `);
  await page.screenshot({ path: join(publicDir, output) });
  await page.close();
}

await renderIcon(180, "apple-touch-icon.png");
await renderIcon(192, "icon-192.png");
await renderIcon(512, "icon-512.png");

const social = await browser.newPage({ viewport: { width: 1200, height: 630 } });
await social.setContent(`
  <style>
    * { box-sizing: border-box; }
    html, body {
      margin: 0;
      width: 100%;
      height: 100%;
      background:
        radial-gradient(circle at 78% 25%, rgba(63, 63, 70, 0.34), transparent 32%),
        #09090b;
      color: #fafafa;
      font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    }
    main {
      width: 100%;
      height: 100%;
      padding: 76px 88px;
      display: flex;
      flex-direction: column;
      justify-content: space-between;
      border: 1px solid #27272a;
    }
    header { display: flex; align-items: center; gap: 24px; font-size: 38px; font-weight: 700; }
    header img { width: 82px; height: 82px; }
    h1 { max-width: 900px; margin: 0; font-size: 72px; line-height: 1.04; letter-spacing: -0.045em; }
    p { margin: 24px 0 0; max-width: 790px; color: #a1a1aa; font-size: 28px; line-height: 1.4; }
    footer { color: #71717a; font-size: 20px; letter-spacing: 0.08em; text-transform: uppercase; }
  </style>
  <main>
    <header><img src="${logoData}" alt="" /><span>Alarvest</span></header>
    <section>
      <h1>Track your portfolio.<br />Rank your strategy.</h1>
      <p>Compare percentage performance and discover public strategies—without connecting a brokerage.</p>
    </section>
    <footer>Compete on strategy, not wealth</footer>
  </main>
`);
await social.screenshot({ path: join(publicDir, "social-card.png") });
await social.close();
await browser.close();
