/*
 * Visual smoke test for the static concepts.
 * Usage: node visual-check.js <node_modules_dir> <output_dir>
 */
const fs = require('fs');
const http = require('http');
const path = require('path');

const nodeModules = process.argv[2];
const outputDir = process.argv[3] || path.join(__dirname, '..', '_screenshots');
const browserExecutable = process.argv[4];
if (!nodeModules) throw new Error('Missing node_modules directory argument.');

const { chromium } = require(path.join(nodeModules, 'playwright'));
const root = path.resolve(__dirname, '..');
const mime = {
  '.html': 'text/html; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.md': 'text/markdown; charset=utf-8'
};

const server = http.createServer((request, response) => {
  const pathname = decodeURIComponent(new URL(request.url, 'http://localhost').pathname);
  const relative = pathname === '/' ? 'index.html' : pathname.replace(/^\/+/, '');
  const target = path.resolve(root, relative);
  if (!target.startsWith(root)) {
    response.writeHead(403).end('Forbidden');
    return;
  }
  fs.readFile(target, (error, data) => {
    if (error) {
      response.writeHead(404).end('Not found');
      return;
    }
    response.writeHead(200, { 'content-type': mime[path.extname(target)] || 'application/octet-stream' });
    response.end(data);
  });
});

const listen = () => new Promise((resolve) => server.listen(0, '127.0.0.1', resolve));

(async () => {
  await listen();
  fs.mkdirSync(outputDir, { recursive: true });
  const port = server.address().port;
  const baseUrl = `http://127.0.0.1:${port}`;
  const browser = await chromium.launch({
    headless: true,
    ...(browserExecutable ? { executablePath: browserExecutable } : {})
  });
  const concepts = fs.readdirSync(root, { withFileTypes: true })
    .filter((entry) => entry.isDirectory() && /^\d{2}-/.test(entry.name))
    .map((entry) => entry.name)
    .sort();
  const issues = [];

  for (const concept of concepts) {
    for (const pageName of ['login', 'home']) {
      for (const scheme of ['light', 'dark']) {
        for (const viewport of [{ name: 'desktop', width: 1440, height: 900 }, { name: 'mobile', width: 390, height: 844 }]) {
          const page = await browser.newPage({ viewport });
          const localIssues = [];
          page.on('pageerror', (error) => localIssues.push(`pageerror: ${error.message}`));
          page.on('console', (message) => {
            if (message.type() === 'error') localIssues.push(`console: ${message.text()}`);
          });
          page.on('response', (response) => {
            if (response.status() >= 400) localIssues.push(`http ${response.status()}: ${response.url()}`);
          });
          await page.goto(`${baseUrl}/${concept}/${pageName}.html?scheme=${scheme}`, { waitUntil: 'networkidle' });
          const metrics = await page.evaluate(() => ({
            appChildren: document.querySelector('#app')?.children.length || 0,
            bodyWidth: document.body.scrollWidth,
            clientWidth: document.documentElement.clientWidth,
            bodyHeight: document.body.scrollHeight,
            title: document.title,
            scheme: document.documentElement.dataset.colorScheme,
            toggleCount: document.querySelectorAll('.theme-toggle').length,
            modeStylesLoaded: [...document.styleSheets].some((sheet) => sheet.href?.endsWith('/shared/modes.css'))
          }));
          if (!metrics.appChildren) localIssues.push('empty #app');
          if (metrics.bodyWidth > metrics.clientWidth + 3) localIssues.push(`horizontal overflow ${metrics.bodyWidth}/${metrics.clientWidth}`);
          if (metrics.bodyHeight < Math.min(600, viewport.height * .8)) localIssues.push(`unexpected short page ${metrics.bodyHeight}px`);
          if (metrics.scheme !== scheme) localIssues.push(`wrong scheme ${metrics.scheme}/${scheme}`);
          if (!metrics.toggleCount) localIssues.push('missing theme toggle');
          if (!metrics.modeStylesLoaded) localIssues.push('modes.css not loaded');
          const screenshotName = `${concept}-${pageName}-${scheme}${viewport.name === 'mobile' ? '-mobile' : ''}.png`;
          await page.screenshot({ path: path.join(outputDir, screenshotName), fullPage: false });
          if (localIssues.length) issues.push({ concept, pageName, scheme, viewport: viewport.name, issues: localIssues });
          await page.close();
        }
      }
    }
  }

  const gallery = await browser.newPage({ viewport: { width: 1440, height: 1100 }, deviceScaleFactor: 1 });
  await gallery.goto(baseUrl, { waitUntil: 'networkidle' });
  await gallery.evaluate(() => localStorage.setItem('file-workshop-color-scheme', 'light'));
  await gallery.reload({ waitUntil: 'networkidle' });
  const loadGalleryPreviews = async () => {
    const frameLocators = gallery.locator('iframe');
    for (let index = 0; index < await frameLocators.count(); index += 1) {
      const frameLocator = frameLocators.nth(index);
      await frameLocator.scrollIntoViewIfNeeded();
      const frameHandle = await frameLocator.elementHandle();
      const contentFrame = await frameHandle.contentFrame();
      await contentFrame.waitForSelector('#app > *', { timeout: 5000 });
    }
    await gallery.evaluate(() => scrollTo(0, 0));
    await gallery.waitForTimeout(400);
  };
  const galleryMetrics = await gallery.evaluate(() => ({
    cards: document.querySelectorAll('[data-folder]').length,
    scheme: document.documentElement.dataset.colorScheme,
    themeButton: Boolean(document.querySelector('.gallery-theme-toggle'))
  }));
  if (galleryMetrics.cards !== concepts.length) issues.push({ concept: 'gallery', issues: [`card count ${galleryMetrics.cards}/${concepts.length}`] });
  if (galleryMetrics.scheme !== 'light') issues.push({ concept: 'gallery', issues: [`initial scheme ${galleryMetrics.scheme}/light`] });
  if (!galleryMetrics.themeButton) issues.push({ concept: 'gallery', issues: ['missing gallery theme toggle'] });
  await loadGalleryPreviews();
  await gallery.locator('#concepts').scrollIntoViewIfNeeded();
  await gallery.waitForTimeout(600);
  await gallery.screenshot({ path: path.join(outputDir, 'gallery-home-light.png'), fullPage: true });
  await gallery.locator('.gallery-theme-toggle').click();
  await loadGalleryPreviews();
  const darkGallery = await gallery.evaluate(() => ({
    scheme: document.documentElement.dataset.colorScheme,
    darkFrames: [...document.querySelectorAll('iframe')].filter((frame) => frame.src.includes('scheme=dark')).length
  }));
  if (darkGallery.scheme !== 'dark') issues.push({ concept: 'gallery', issues: [`toggled scheme ${darkGallery.scheme}/dark`] });
  if (darkGallery.darkFrames !== concepts.length) issues.push({ concept: 'gallery', issues: [`dark previews ${darkGallery.darkFrames}/${concepts.length}`] });
  await gallery.screenshot({ path: path.join(outputDir, 'gallery-home-dark.png'), fullPage: true });
  await gallery.locator('[data-preview-page="login"]').click();
  await loadGalleryPreviews();
  await gallery.screenshot({ path: path.join(outputDir, 'gallery-login-dark.png'), fullPage: true });
  await gallery.locator('.gallery-theme-toggle').click();
  await loadGalleryPreviews();
  await gallery.screenshot({ path: path.join(outputDir, 'gallery-login-light.png'), fullPage: true });
  await gallery.close();

  await browser.close();
  server.close();
  const report = { concepts: concepts.length, pagesChecked: concepts.length * 2 * 2 * 2, issues };
  fs.writeFileSync(path.join(outputDir, 'report.json'), JSON.stringify(report, null, 2));
  console.log(JSON.stringify(report, null, 2));
  if (issues.length) process.exitCode = 1;
})().catch((error) => {
  console.error(error);
  server.close();
  process.exitCode = 1;
});
