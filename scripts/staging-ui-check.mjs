#!/usr/bin/env node
// No traces, HARs, storage-state files, request bodies, or raw browser diagnostics.
import { readFile, mkdir, writeFile, access } from 'node:fs/promises';
import path from 'node:path';

const DEFAULT_ROUTES = ['/', '/setup', '/orders', '/catalogue', '/inventory', '/stores', '/customers', '/settings/payments', '/assistant', '/journeys', '/assistant/ai-chat', '/conversations', '/knowledge', '/team', '/settings/business', '/settings/whatsapp'];
const secrets = new Set();
const tokenPattern = /\b(?:Bearer\s+\S+|eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+(?:\.[A-Za-z0-9_-]+)?|[A-Za-z0-9_+/-]{40,}={0,2})\b/gi;
function clean(value) {
  let text = String(value ?? '');
  for (const secret of secrets) if (secret) text = text.split(secret).join('[REDACTED]');
  return text.replace(tokenPattern, '[REDACTED]');
}
function pathname(url) {
  try { return clean(new URL(url).pathname); } catch { return '[unavailable]'; }
}
const out = path.resolve(process.env.OUT_DIR || `.staging-evidence/${new Date().toISOString().replaceAll(':', '-')}`);
const summary = { startedAt: new Date().toISOString(), organizations: {}, results: [], notes: [
  'Console messages and page-error messages are omitted for confidentiality; locations/names and counts are retained.',
  'Request/response bodies, headers, query strings, auth state and browser traces are never recorded.',
  'Hidden desktop journey tab navigation is temporarily revealed for tab captures.',
] };
let browser;
let stage = 'configuration';
let active;
let page;
async function rememberSecrets() {
  if (!page) return;
  // Learn opaque auth values only in memory so even a UI accidentally echoing one is masked.
  const values = await page.evaluate(() => {
    const found = [];
    function visit(value, key = '') {
      if (typeof value === 'string') {
        if (/token|authorization|secret|password/i.test(key)) found.push(value);
        try { visit(JSON.parse(value), key); } catch { /* ordinary string */ }
      } else if (value && typeof value === 'object') {
        for (const [k, v] of Object.entries(value)) visit(v, k);
      }
    }
    for (const store of [localStorage, sessionStorage]) {
      for (let i = 0; i < store.length; i++) { const key = store.key(i); visit(store.getItem(key), key); }
    }
    return found;
  }).catch(() => []);
  for (const value of values) secrets.add(value);
  for (const cookie of await page.context().cookies()) secrets.add(cookie.value);
}
async function settle() {
  await page.waitForLoadState('domcontentloaded').catch(() => {});
  await page.waitForLoadState('networkidle', { timeout: 8000 }).catch(() => {});
  await page.waitForTimeout(750);
  await rememberSecrets();
}
function start(route, width) {
  active = { route, width, consoleErrors: [], pageErrors: [], failedRequests: [], blockedRequests: [] };
  summary.results.push(active);
  return active;
}
async function capture(row) {
  await settle();
  if (/^\/login\/?$/.test(new URL(page.url()).pathname)) throw new Error('Session returned to /login');
  row.actualPath = pathname(page.url());
  const dimensions = await page.evaluate(() => ({ scrollWidth: document.documentElement.scrollWidth, clientWidth: document.documentElement.clientWidth }));
  row.overflow = { ...dimensions, detected: dimensions.scrollWidth > dimensions.clientWidth };
  // Store only matched literals, never arbitrary surrounding text.
  row.flaggedText = [...new Set((await page.locator('body').innerText()).match(/Loading provider capabilities|undefined|NaN|\[object Object\]/g) || [])];
  await page.evaluate(({ values }) => {
    const token = /\b(?:Bearer\s+\S+|eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+(?:\.[A-Za-z0-9_-]+)?|[A-Za-z0-9_+/-]{40,}={0,2})\b/i;
    for (const el of document.querySelectorAll('body *')) {
      const directText = [...el.childNodes].filter(n => n.nodeType === Node.TEXT_NODE).map(n => n.textContent).join('');
      const value = 'value' in el ? String(el.value) : '';
      if (el.matches('input[type=password]') || token.test(directText + ' ' + value) || values.some(s => s && (directText.includes(s) || value.includes(s)))) el.setAttribute('data-staging-secret', '');
    }
  }, { values: [...secrets] });
  row.screenshot = `${String(summary.results.indexOf(row) + 1).padStart(3, '0')}-${row.width}.png`;
  try {
    await page.screenshot({ path: path.join(out, row.screenshot), fullPage: true, mask: [page.locator('[data-staging-secret]')], maskColor: '#000' });
  } finally {
    await page.locator('[data-staging-secret]').evaluateAll(nodes => nodes.forEach(n => n.removeAttribute('data-staging-secret')));
  }
}
async function organizations() {
  const info = { headerName: null, shellName: null, switcherPresent: false, organizations: [] };
  const header = page.locator('header.topbar, header[role=banner], [role=banner]').first();
  const name = header.locator('[data-testid*="org"], .organization-name, .org-name');
  if (await name.count()) info.headerName = clean(await name.first().innerText());
  const subtitle = page.locator('.sidebar .zidi-logo__subtitle, .sidebar [class*="subtitle"]');
  if (await subtitle.count()) info.shellName = clean(await subtitle.first().innerText());
  const switcher = page.getByRole('button', { name: /switch (organization|organisation|workspace)|select (organization|organisation|workspace)|organization switcher|organisation switcher/i }).first();
  if (await switcher.isVisible()) {
    info.switcherPresent = true;
    await switcher.click();
    await settle();
    info.organizations = (await page.locator('[role=option]:visible, [role=menuitem]:visible').allInnerTexts()).map(clean);
    await page.keyboard.press('Escape');
  }
  // Current shell exposes the organization through its read-only account menu.
  if (!info.headerName && await page.getByRole('button', { name: 'Open account menu', exact: true }).isVisible()) {
    await page.getByRole('button', { name: 'Open account menu', exact: true }).click();
    const detail = page.locator('.shell-panel p, [role=dialog] p');
    const texts = await detail.allInnerTexts();
    const org = texts.find(t => t.includes(' · '));
    if (org) info.shellName = clean(org.split(' · ').slice(1).join(' · '));
    await page.keyboard.press('Escape');
    const close = page.getByRole('button', { name: 'Close dialog', exact: true });
    if (await close.isVisible()) await close.click();
    // Navigating closes panels even if Escape is unsupported.
    await page.goto(new URL('/', base).href);
    await settle();
  }
  if (!info.headerName) info.note = 'No organization name element found in the shell header; shellName is the sidebar/account fallback.';
  summary.organizations = info;
}
let base;
try {
  // Disable Playwright API debug logging, which could include fill arguments.
  delete process.env.DEBUG;
  delete process.env.PWDEBUG;
  base = new URL(process.env.ADMIN_URL || 'https://zidicommerce-staging-admin-staging.up.railway.app');
  if (!['https:', 'http:'].includes(base.protocol) || base.username || base.password) throw new Error();
  const routes = process.env.ROUTES === undefined ? DEFAULT_ROUTES : process.env.ROUTES.split(',').map(s => s.trim()).filter(Boolean);
  if (!routes.length || routes.some(r => !r.startsWith('/') || r.startsWith('//') || new URL(r, base).origin !== base.origin || /[?#]/.test(r))) throw new Error();
  const widths = (process.env.WIDTHS || '390,768,1440').split(',').map(Number);
  if (widths.some(w => !Number.isInteger(w) || w < 240 || w > 7680)) throw new Error();
  const login = await readFile(process.env.LOGIN_FILE || '/Users/oluwateniolasobande/.zidi-staging-login.YZZyMV/login.txt', 'utf8');
  const email = login.match(/^Email:\s*(.+)$/mi)?.[1].trim();
  const password = login.match(/^Password:[ \t]*(.+)$/mi)?.[1].trim();
  if (!email || !password) throw new Error();
  secrets.add(password);
  const { chromium } = await import('playwright');
  try { await access(chromium.executablePath()); } catch {
    console.error('Chromium is not installed. Run: npx playwright install chromium');
    process.exitCode = 2;
    process.exit();
  }
  stage = 'browser setup';
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 }, serviceWorkers: 'block' });
  // Defense in depth: after login, all network mutations are blocked, including background writes.
  let loginAllowed = true;
  await context.route('**/*', async route => {
    const request = route.request();
    const allowed = ['GET', 'HEAD', 'OPTIONS'].includes(request.method()) || (loginAllowed && request.method() === 'POST' && new URL(request.url()).origin !== 'null' && /\/auth\/login\/?$/.test(new URL(request.url()).pathname));
    if (allowed) await route.continue();
    else { active?.blockedRequests.push({ path: pathname(request.url()), status: null }); await route.abort(); }
  });
  page = await context.newPage();
  page.setDefaultTimeout(15000);
  page.on('console', message => {
    if (active && message.type() === 'error') active.consoleErrors.push({ path: pathname(message.location().url), line: message.location().lineNumber, message: '[omitted: may contain sensitive data]' });
  });
  page.on('pageerror', () => active?.pageErrors.push({ message: '[omitted: may contain sensitive data]' }));
  page.on('response', response => { if (active && response.status() >= 400) active.failedRequests.push({ path: pathname(response.url()), status: response.status() }); });
  page.on('requestfailed', request => active?.failedRequests.push({ path: pathname(request.url()), status: null }));
  await mkdir(out, { recursive: true, mode: 0o700 });
  stage = 'login';
  await page.goto(new URL('/login', base).href);
  await page.getByLabel('Email', { exact: true }).fill(email);
  await page.getByLabel('Password', { exact: true }).fill(password);
  await page.getByRole('button', { name: 'Sign in', exact: true }).click();
  try { await page.waitForURL(url => !/^\/login\/?$/.test(url.pathname), { timeout: 30000 }); } catch {
    await rememberSecrets();
    summary.loginError = clean((await page.locator('[role=alert]:visible, .error-text:visible').allInnerTexts()).join('\n')) || 'Still on /login; no visible error text.';
    throw new Error();
  }
  loginAllowed = false;
  await settle();
  const tour = page.locator('[role=dialog][aria-labelledby="tour-title"]');
  if (await tour.isVisible().catch(() => false)) {
    summary.productTour = { shownOnFirstVisit: true, title: clean(await page.locator('#tour-title').innerText().catch(() => '')) };
    await page.screenshot({ path: path.join(out, '000-tour.png') });
    await tour.getByRole('button', { name: 'Skip tour' }).click();
    await settle();
  } else summary.productTour = { shownOnFirstVisit: false };
  await organizations();
  stage = 'capture';
  for (const width of widths) {
    await page.setViewportSize({ width, height: 1000 });
    for (const route of routes) {
      const row = start(route, width);
      try { await page.goto(new URL(route, base).href); await capture(row); }
      catch { row.captureError = 'Navigation or capture failed (raw diagnostics omitted).'; if (/^\/login\/?$/.test(new URL(page.url()).pathname)) { stage = 'login/session'; throw new Error(); } }
    }
    const row = start('/journeys [first Open]', width);
    try {
      await page.goto(new URL('/journeys', base).href);
      await settle();
      const open = page.getByRole('button', { name: 'Open', exact: true }).first();
      if (!await open.isVisible()) { row.skipped = 'No journey Open button available; no journey was created.'; await capture(row); continue; }
      await open.click();
      await settle();
      await capture(row);
      for (const tab of ['Canvas', 'Steps', 'Edit', 'Preview']) {
        const tabRow = start(`/journeys [${tab}]`, width);
        try {
          const target = page.getByRole('tab', { name: tab, exact: true });
          if (!await target.isVisible()) await page.addStyleTag({ content: '.journey-mobile-tabs { display: block !important; }' });
          await target.click();
          await capture(tabRow);
          if (tab === 'Preview') {
            const flow = page.getByRole('button', { name: 'Flow', exact: true });
            if (await flow.isVisible()) {
              const flowRow = start('/journeys [Preview / Flow]', width);
              await flow.click();
              await capture(flowRow);
            }
          }
        } catch { active.captureError = 'Tab unavailable or capture failed (raw diagnostics omitted).'; }
      }
    } catch { active.captureError = 'Journey unavailable or capture failed (raw diagnostics omitted).'; }
  }
} catch {
  summary.failure = `Failed during ${stage}. Check configuration, login form/access, and local Playwright installation.`;
  process.exitCode = 1;
} finally {
  if (browser) await browser.close().catch(() => {});
}
summary.finishedAt = new Date().toISOString();
try {
  await mkdir(out, { recursive: true, mode: 0o700 });
  await writeFile(path.join(out, 'summary.json'), JSON.stringify(summary, (_key, value) => typeof value === 'string' ? clean(value) : value, 2), { mode: 0o600 });
  const escape = value => clean(value).replaceAll('|', '\\|').replaceAll('\n', ' ');
  const lines = ['# Staging UI evidence', '', ...(summary.failure ? [summary.failure, '', escape(summary.loginError || ''), ''] : []), `Organization: ${escape(summary.organizations.headerName || summary.organizations.shellName || 'Unavailable')}`, '', '| Route | Width | Console errors | Failed requests | Overflow | Flagged text |', '| --- | ---: | ---: | ---: | --- | --- |'];
  for (const row of summary.results) lines.push(`| ${escape(row.route)} | ${row.width} | ${row.consoleErrors.length} | ${row.failedRequests.length} | ${row.overflow?.detected ?? 'unknown'} | ${escape(row.flaggedText?.join(', ') || '')}${row.captureError ? ' (capture failed)' : ''}${row.skipped ? ' (skipped)' : ''} |`);
  lines.push('', 'See summary.json for page errors, skipped checks, blocked writes, and capture failures.', '', ...summary.notes);
  await writeFile(path.join(out, 'summary.md'), lines.join('\n') + '\n', { mode: 0o600 });
  console.log(summary.failure ? `Staging UI check failed during ${stage}; see summary.json.` : 'Staging UI evidence capture completed; see OUT_DIR summary.json and summary.md.');
} catch {
  console.error('Could not write staging evidence summaries. Check OUT_DIR permissions.');
  process.exitCode = 1;
}
