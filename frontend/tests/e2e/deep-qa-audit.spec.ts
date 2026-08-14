/*
 * Deep E2E QA audit for Parts Engine.
 *
 * Runs real (non-mocked) user journeys through the UI and records:
 *   - screenshots at every step
 *   - HAR (HTTP archive) for the whole session
 *   - Playwright trace file
 *   - per-step JSON verdict (pass / fail / soft-fail)
 *
 * Target selection:
 *   E2E_BASE_URL       = http://127.0.0.1:8080  (default local)
 *                      | https://qa.ifritah.com (production)
 *   E2E_TARGET_LABEL   = "local" | "prod"      (used in artifact folder names)
 *
 * Artifacts land in:
 *   ../qa/e2e-report/<target>/
 *     screenshots/*.png
 *     session.har
 *     trace.zip (via playwright's built-in trace, when enabled)
 *     findings.json
 *
 * We deliberately use test.step() so the CLI shows a live tree of what's
 * happening. Assertions inside try/catch record a soft-fail into the JSON
 * instead of aborting — we want a full pass, not the first red step.
 */

import { test, type Page, expect } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';
import { fileURLToPath } from 'url';

type Finding = {
  id: string;
  category: string;
  step: string;
  ok: boolean;
  soft: boolean;
  message: string;
  elapsedMs?: number;
  screenshot?: string;
  metric?: Record<string, unknown>;
};

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const TARGET_LABEL = process.env.E2E_TARGET_LABEL || (process.env.E2E_BASE_URL?.includes('ifritah.com') ? 'prod' : 'local');
const ARTIFACT_ROOT = path.resolve(__dirname, '..', '..', '..', 'qa', 'e2e-report', TARGET_LABEL);
const SCREENSHOT_DIR = path.join(ARTIFACT_ROOT, 'screenshots');
const FINDINGS_PATH = path.join(ARTIFACT_ROOT, 'findings.json');
const HAR_PATH = path.join(ARTIFACT_ROOT, 'session.har');

function ensureDir(p: string) {
  if (!fs.existsSync(p)) fs.mkdirSync(p, { recursive: true });
}
ensureDir(SCREENSHOT_DIR);

const findings: Finding[] = [];

function record(f: Finding) {
  findings.push(f);
}

async function shot(page: Page, name: string): Promise<string> {
  const file = path.join(SCREENSHOT_DIR, `${name}.png`);
  await page.screenshot({ path: file, fullPage: true }).catch(() => undefined);
  return path.relative(ARTIFACT_ROOT, file);
}

async function safeStep<T>(
  id: string,
  category: string,
  step: string,
  fn: () => Promise<T>,
  soft = true
): Promise<T | undefined> {
  const start = Date.now();
  try {
    const result = await fn();
    record({ id, category, step, ok: true, soft, message: 'ok', elapsedMs: Date.now() - start });
    return result;
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err);
    record({
      id,
      category,
      step,
      ok: false,
      soft,
      message: msg.substring(0, 500),
      elapsedMs: Date.now() - start,
    });
    if (!soft) throw err;
    return undefined;
  }
}

/*
 * Playwright test.describe.serial ensures single-worker sequential execution.
 * We rely on it so screenshots and HAR reflect a coherent narrative.
 */

test.describe.configure({ mode: 'serial' });

// Top-level test.use — Playwright requires trace/recordHar to be set
// outside a describe group.
test.use({
  recordHar: { path: HAR_PATH, mode: 'full' as any },
  ignoreHTTPSErrors: true,
  viewport: { width: 1440, height: 900 },
  trace: 'on',
});

test.describe('Deep QA audit — real user journeys', () => {
  test.beforeAll(() => {
    findings.push({
      id: 'setup-00',
      category: 'setup',
      step: `target=${TARGET_LABEL} baseURL=${process.env.E2E_BASE_URL || 'default'}`,
      ok: true,
      soft: false,
      message: `Artifact root: ${ARTIFACT_ROOT}`,
    });
  });

  test.afterAll(async () => {
    fs.writeFileSync(FINDINGS_PATH, JSON.stringify(findings, null, 2), 'utf8');
  });

  // ─────────────────────────────────────────────────────────────
  // LANDING / NAVIGATION
  // ─────────────────────────────────────────────────────────────
  test('L1 landing loads + navigation works', async ({ page }) => {
    const consoleErrors: string[] = [];
    page.on('console', (msg) => {
      if (msg.type() === 'error') consoleErrors.push(msg.text());
    });
    page.on('pageerror', (err) => consoleErrors.push(`pageerror: ${err.message}`));

    await safeStep('L1-01', 'landing', 'GET /', async () => {
      const t0 = Date.now();
      const resp = await page.goto('/', { waitUntil: 'domcontentloaded' });
      record({
        id: 'L1-01-metric',
        category: 'landing',
        step: 'initial DCL timing',
        ok: true,
        soft: true,
        message: `HTTP ${resp?.status()}`,
        elapsedMs: Date.now() - t0,
      });
      expect(resp?.status()).toBe(200);
    }, false);

    await safeStep('L1-02', 'landing', 'header title visible', async () => {
      await expect(page.getByRole('heading', { name: 'Parts Engine' })).toBeVisible({ timeout: 8000 });
    });

    await safeStep('L1-03', 'landing', 'evidence-first banner visible', async () => {
      await expect(page.getByText(/Evidence-first Hyundai \/ Kia parts workflow/i)).toBeVisible({ timeout: 4000 });
    });

    await safeStep('L1-04', 'landing', 'has 3 nav tabs (VIN / Search / Catalog)', async () => {
      const links = ['VIN decode', 'Search', 'Catalog'];
      for (const l of links) {
        await expect(page.getByRole('link', { name: l })).toBeVisible();
      }
    });

    const shot1 = await shot(page, '01-landing');
    findings[findings.length - 1].screenshot = shot1;

    await safeStep('L1-05', 'landing', 'nav to Search tab', async () => {
      await page.getByRole('link', { name: 'Search' }).click();
      await expect(page).toHaveURL(/\/oem/);
      await expect(page.getByLabel(/OEM \/ Part Number \/ Description/i)).toBeVisible();
    });
    (findings[findings.length - 1].screenshot = await shot(page, '02-search-tab'));

    await safeStep('L1-06', 'landing', 'nav to Catalog tab', async () => {
      await page.getByRole('link', { name: 'Catalog' }).click();
      await expect(page).toHaveURL(/\/catalog/);
    });
    (findings[findings.length - 1].screenshot = await shot(page, '03-catalog-tab'));

    await safeStep('L1-07', 'landing', 'no console errors during nav', async () => {
      expect(consoleErrors, `console errors: ${consoleErrors.join(' | ')}`).toEqual([]);
    });
  });

  // ─────────────────────────────────────────────────────────────
  // VIN JOURNEY — golden Tucson 2016
  // ─────────────────────────────────────────────────────────────
  test('V1 VIN journey — 2016 Tucson (golden)', async ({ page }) => {
    await page.goto('/');
    await shot(page, '10-vin-landing');

    const t0 = Date.now();
    await safeStep('V1-01', 'vin', 'type VIN + click Decode', async () => {
      await page.getByLabel('VIN (17 characters)').fill('KM8J33A46GU123456');
      await page.getByRole('button', { name: 'Decode' }).click();
    });

    // Wait for either variants or an error banner.
    const decodeResult = await safeStep('V1-02', 'vin', 'variants appear (needsConfirmation)', async () => {
      await Promise.race([
        page.getByTestId('vin-catalog-matches').waitFor({ state: 'visible', timeout: 30_000 }),
        page.getByText(/Failed to decode/i).waitFor({ state: 'visible', timeout: 30_000 }),
      ]);
      const failed = await page.getByText(/Failed to decode/i).isVisible().catch(() => false);
      if (failed) throw new Error('decode returned an error banner');
      const cardCount = await page.getByTestId('vin-match-card').count();
      return { cardCount, elapsedMs: Date.now() - t0 };
    });
    record({
      id: 'V1-02-metric',
      category: 'vin',
      step: 'variants',
      ok: !!decodeResult,
      soft: true,
      message: `variants=${decodeResult?.cardCount ?? 'n/a'}`,
      metric: decodeResult,
    });
    await shot(page, '11-vin-decoded');

    await safeStep('V1-03', 'vin', 'recall banner visible for Tucson 2016', async () => {
      const recall = page.getByTestId('recall-banner');
      await expect(recall).toBeVisible({ timeout: 4000 });
      await expect(recall).toContainText(/recall/i);
    });
    await shot(page, '12-vin-recall');

    await safeStep('V1-04', 'vin', 'confirm variant → open catalog', async () => {
      await page.getByTestId('vin-open-catalog').first().click();
      await expect(page).toHaveURL(/\/catalog\?/);
      await expect(page.getByTestId('catalog-source-banner')).toContainText(/KM8J33A46GU123456/i);
    });
    await shot(page, '13-vin-catalog');
  });

  // ─────────────────────────────────────────────────────────────
  // VIN JOURNEY — invalid input
  // ─────────────────────────────────────────────────────────────
  test('V2 VIN validation — invalid + non-existent', async ({ page }) => {
    await page.goto('/');
    await safeStep('V2-01', 'vin', 'button disabled for invalid VIN', async () => {
      await page.getByLabel('VIN (17 characters)').fill('NOT-A-VIN');
      const btn = page.getByRole('button', { name: 'Decode' });
      await expect(btn).toBeDisabled();
    });
    await shot(page, '20-vin-invalid');

    await safeStep('V2-02', 'vin', 'nonexistent VIN 17-char decodes without crash', async () => {
      await page.getByLabel('VIN (17 characters)').fill('ZZZZZZZZZZZZZZZZZ');
      const btn = page.getByRole('button', { name: 'Decode' });
      if (await btn.isEnabled()) {
        await btn.click();
        // Whatever it returns, the page must not blank out
        await expect(page.locator('main')).toBeVisible({ timeout: 30_000 });
      }
    });
    await shot(page, '21-vin-nonexistent');
  });

  // ─────────────────────────────────────────────────────────────
  // OEM SEARCH — golden Hyundai/Kia
  // ─────────────────────────────────────────────────────────────
  test('O1 OEM search — golden 26300-35505 (Hyundai oil filter)', async ({ page }) => {
    await page.goto('/oem');
    await shot(page, '30-oem-landing');

    const t0 = Date.now();
    await safeStep('O1-01', 'oem', 'type + click Search', async () => {
      await page.getByLabel(/OEM \/ Part Number \/ Description/i).fill('26300-35505');
      await page.getByRole('button', { name: 'Search' }).click();
    });

    const firstArticle = await safeStep('O1-02', 'oem', 'result cards visible', async () => {
      await page.getByTestId('search-result-card').first().waitFor({ state: 'visible', timeout: 30_000 });
      const articles = await page.getByTestId('search-result-article').allInnerTexts();
      return { articles, elapsedMs: Date.now() - t0 };
    });
    record({
      id: 'O1-02-metric',
      category: 'oem',
      step: 'articles + timing',
      ok: !!firstArticle,
      soft: true,
      message: `articles=${JSON.stringify(firstArticle?.articles ?? [])}`,
      metric: firstArticle,
    });
    await shot(page, '31-oem-26300-results');

    await safeStep('O1-03', 'oem', 'exact HK OEM ranks first', async () => {
      const first = (await page.getByTestId('search-result-article').first().innerText()).replace(/\s+/g, '');
      expect(first.toUpperCase()).toContain('26300-35505'.toUpperCase().replace(/-/g, '').substring(0, 5));
      expect(first).toMatch(/26300/);
    });

    await safeStep('O1-04', 'oem', 'click into detail modal', async () => {
      await page.getByTestId('search-result-card').first().click();
      await expect(page.getByText(/Evidence-first part detail/i)).toBeVisible({ timeout: 8000 });
      await expect(page.getByText(/^Source:/)).toBeVisible();
      await expect(page.getByText(/^Confidence:/)).toBeVisible();
    });
    await shot(page, '32-oem-detail-modal');
  });

  test('O2 OEM search — golden 97133-D3000 (Tucson cabin filter)', async ({ page }) => {
    await page.goto('/oem');
    const t0 = Date.now();
    await page.getByLabel(/OEM \/ Part Number \/ Description/i).fill('97133-D3000');
    await page.getByRole('button', { name: 'Search' }).click();
    const articles = await safeStep('O2-01', 'oem', 'result cards visible', async () => {
      await page.getByTestId('search-result-card').first().waitFor({ state: 'visible', timeout: 30_000 });
      return { articles: await page.getByTestId('search-result-article').allInnerTexts(), elapsedMs: Date.now() - t0 };
    });
    record({
      id: 'O2-metric', category: 'oem', step: '97133-D3000 articles + timing',
      ok: !!articles, soft: true, message: JSON.stringify(articles), metric: articles,
    });
    await shot(page, '33-oem-97133-results');
  });

  // ─────────────────────────────────────────────────────────────
  // OEM SEARCH — real HK OEM not in seed catalog
  // ─────────────────────────────────────────────────────────────
  test('O3 OEM search — real Kia OEM 54528-4A100 (not in seed)', async ({ page }) => {
    await page.goto('/oem');
    const t0 = Date.now();
    await page.getByLabel(/OEM \/ Part Number \/ Description/i).fill('54528-4A100');
    await page.getByRole('button', { name: 'Search' }).click();

    const state = await safeStep('O3-01', 'oem', 'settle (result or empty-state)', async () => {
      await Promise.race([
        page.getByTestId('search-result-card').first().waitFor({ state: 'visible', timeout: 30_000 }),
        page.getByText(/no results|no matches/i).waitFor({ state: 'visible', timeout: 30_000 }),
      ]).catch(() => undefined);
      const cardCount = await page.getByTestId('search-result-card').count();
      const bodyText = await page.locator('main').innerText();
      return { cardCount, hasEmptyMessage: /no results|no matches|not found/i.test(bodyText), elapsedMs: Date.now() - t0 };
    });
    record({
      id: 'O3-metric', category: 'oem', step: '54528-4A100 state',
      ok: !!state, soft: true, message: JSON.stringify(state), metric: state,
    });
    await shot(page, '34-oem-54528-state');
  });

  // ─────────────────────────────────────────────────────────────
  // OEM SEARCH — Toyota boundary (must NOT return fake HK part)
  // ─────────────────────────────────────────────────────────────
  test('O4 OEM search — Toyota 90915-YZZE1 boundary', async ({ page }) => {
    await page.goto('/oem');
    const t0 = Date.now();
    await page.getByLabel(/OEM \/ Part Number \/ Description/i).fill('90915-YZZE1');
    await page.getByRole('button', { name: 'Search' }).click();

    const state = await safeStep('O4-01', 'oem', 'settle', async () => {
      await Promise.race([
        page.getByTestId('search-result-card').first().waitFor({ state: 'visible', timeout: 30_000 }),
        page.getByText(/Hyundai\/Kia parts only|not in scope|Toyota/i).waitFor({ state: 'visible', timeout: 30_000 }),
      ]).catch(() => undefined);
      const cardCount = await page.getByTestId('search-result-card').count();
      const body = await page.locator('main').innerText();
      const mentionsToyota = /Toyota/i.test(body);
      const mentionsOnlyHK = /Hyundai\/Kia parts only/i.test(body);
      return { cardCount, mentionsToyota, mentionsOnlyHK, elapsedMs: Date.now() - t0 };
    });
    record({
      id: 'O4-metric', category: 'oem', step: '90915-YZZE1 boundary',
      ok: !!state, soft: true, message: JSON.stringify(state), metric: state,
    });
    await shot(page, '35-oem-toyota-boundary');

    await safeStep('O4-02', 'oem', 'no fabricated Toyota result surfaced', async () => {
      // Even if a card appears, description must NOT be a scraped chrome string.
      const cards = await page.getByTestId('search-result-card').count();
      if (cards === 0) return;
      const descriptions = await page.getByTestId('search-result-card').allInnerTexts();
      const joined = descriptions.join(' | ').toLowerCase();
      const junky = ['sign up', 'log in', 'click here', 'life-time-filter'];
      for (const j of junky) {
        expect(joined, `first result should not contain scrape junk "${j}"`).not.toContain(j);
      }
    });
  });

  // ─────────────────────────────────────────────────────────────
  // TEXT SEARCH — common consumer terms
  // ─────────────────────────────────────────────────────────────
  test('T1 text search — cabin air filter', async ({ page }) => {
    await page.goto('/oem');
    const t0 = Date.now();
    await page.getByLabel(/OEM \/ Part Number \/ Description/i).fill('cabin air filter');
    await page.getByRole('button', { name: 'Search' }).click();
    const first = await safeStep('T1-01', 'text', 'settle', async () => {
      await page.getByTestId('search-result-card').first().waitFor({ state: 'visible', timeout: 30_000 });
      const articles = await page.getByTestId('search-result-article').allInnerTexts();
      return { articles, elapsedMs: Date.now() - t0 };
    });
    record({
      id: 'T1-metric', category: 'text', step: 'cabin air filter articles',
      ok: !!first, soft: true, message: JSON.stringify(first), metric: first,
    });
    await shot(page, '40-text-cabin-air-filter');
  });

  test('T2 text search — oil filter', async ({ page }) => {
    await page.goto('/oem');
    const t0 = Date.now();
    await page.getByLabel(/OEM \/ Part Number \/ Description/i).fill('oil filter');
    await page.getByRole('button', { name: 'Search' }).click();
    const first = await safeStep('T2-01', 'text', 'settle', async () => {
      await page.getByTestId('search-result-card').first().waitFor({ state: 'visible', timeout: 30_000 });
      const articles = await page.getByTestId('search-result-article').allInnerTexts();
      const brands = await page.getByTestId('search-result-brand').allInnerTexts().catch(() => [] as string[]);
      return { articles, brands, elapsedMs: Date.now() - t0 };
    });
    record({
      id: 'T2-metric', category: 'text', step: 'oil filter articles',
      ok: !!first, soft: true, message: JSON.stringify(first), metric: first,
    });
    await shot(page, '41-text-oil-filter');
  });

  // ─────────────────────────────────────────────────────────────
  // CATALOG — direct URL with vehicle context
  // ─────────────────────────────────────────────────────────────
  test('C1 catalog — direct 2016 Tucson (linkageTargetId=10001)', async ({ page }) => {
    const t0 = Date.now();
    await page.goto('/catalog?make=HYUNDAI&model=TUCSON&vehicleId=10001&sourceType=e2e&sourceQuery=deep-audit');
    await safeStep('C1-01', 'catalog', 'catalog banner + groups', async () => {
      await expect(page.getByTestId('catalog-source-banner')).toBeVisible({ timeout: 10_000 });
    });
    const t1 = Date.now();
    await safeStep('C1-02', 'catalog', 'all-parts click surfaces at least one part', async () => {
      await page.getByTestId('catalog-all-parts').click();
      await page.getByTestId('catalog-part-article').first().waitFor({ state: 'visible', timeout: 30_000 });
    });
    await shot(page, '50-catalog-all-parts');
    await safeStep('C1-03', 'catalog', 'part detail opens', async () => {
      await page.getByTestId('catalog-part-article').first().click();
      await expect(page.getByText(/^Source:/)).toBeVisible({ timeout: 8000 });
      await expect(page.getByText(/^Confidence:/)).toBeVisible();
    });
    await shot(page, '51-catalog-detail');
    record({
      id: 'C1-metric', category: 'catalog', step: 'time to detail',
      ok: true, soft: true, message: `catalog-open + click-through`,
      metric: { catalogMs: t1 - t0, detailMs: Date.now() - t1 },
    });
  });

  // ─────────────────────────────────────────────────────────────
  // ROUTING / ERROR SURFACE
  // ─────────────────────────────────────────────────────────────
  test('E1 unknown /api/* returns JSON 404 (not SPA HTML)', async ({ page }) => {
    const resp = await page.request.get('/api/vin/KM8J33A46GU123456', { failOnStatusCode: false });
    const ct = resp.headers()['content-type'] || '';
    const body = await resp.text();
    record({
      id: 'E1-metric', category: 'routing', step: 'GET /api/vin/:vin',
      ok: resp.status() === 404 && /json/i.test(ct),
      soft: true,
      message: `status=${resp.status()} content-type="${ct}" body-preview="${body.slice(0, 120)}"`,
    });
  });

  test('E2 unknown non-API route serves SPA (React Router picks it up)', async ({ page }) => {
    const resp = await page.goto('/no-such-route-xyz', { waitUntil: 'domcontentloaded' });
    record({
      id: 'E2-metric', category: 'routing', step: 'GET /no-such-route-xyz',
      ok: resp?.status() === 200,
      soft: true,
      message: `status=${resp?.status()}`,
    });
    await shot(page, '60-unknown-route');
  });

  // ─────────────────────────────────────────────────────────────
  // PERFORMANCE — landing + a heavy search
  // ─────────────────────────────────────────────────────────────
  test('P1 performance — landing + one heavy search', async ({ page }) => {
    const nav = [] as { name: string; ms: number }[];
    let dclStart = Date.now();
    await page.goto('/', { waitUntil: 'domcontentloaded' });
    nav.push({ name: 'landing-dcl', ms: Date.now() - dclStart });

    dclStart = Date.now();
    await page.goto('/oem', { waitUntil: 'domcontentloaded' });
    nav.push({ name: 'oem-dcl', ms: Date.now() - dclStart });

    dclStart = Date.now();
    await page.getByLabel(/OEM \/ Part Number \/ Description/i).fill('26300-35505');
    await page.getByRole('button', { name: 'Search' }).click();
    await page.getByTestId('search-result-card').first().waitFor({ state: 'visible', timeout: 60_000 });
    nav.push({ name: 'golden-oem-search', ms: Date.now() - dclStart });

    record({
      id: 'P1-metric', category: 'perf', step: 'landing + golden search',
      ok: true, soft: true, message: JSON.stringify(nav), metric: { timings: nav },
    });
  });
});
