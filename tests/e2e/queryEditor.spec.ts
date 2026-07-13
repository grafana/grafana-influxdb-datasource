import { expect, test } from '@grafana/plugin-e2e';
import { type Page } from '@playwright/test';

const PLUGIN_TYPE = 'influxdb';
const PROVISIONED_FILE = 'datasources.yml';

function exploreUrl(uid: string, opts?: { rawSql?: string; format?: string }): string {
  const query: Record<string, unknown> = {
    refId: 'A',
    datasource: { type: PLUGIN_TYPE, uid },
    dataset: 'iox',
    format: opts?.format ?? 'table',
  };
  if (opts?.rawSql) {
    query.rawSql = opts.rawSql;
  }
  const panes = JSON.stringify({
    explore: {
      datasource: uid,
      queries: [query],
      range: { from: 'now-4h', to: 'now' },
    },
  });
  return `/explore?orgId=1&schemaVersion=1&panes=${encodeURIComponent(panes)}`;
}

async function switchEditorMode(page: Page, mode: 'Builder' | 'Code'): Promise<void> {
  const target = page.getByRole('radio', { name: mode, exact: true });
  if (await target.isChecked()) {
    return;
  }
  await target.click();
  if (mode === 'Builder') {
    const discardButton = page.getByRole('button', { name: 'Discard code and switch' })
    if (await discardButton.isVisible({ timeout: 3000 }).catch(() => false)) {
      await discardButton.click();
    }
  }
  await expect(target).toBeChecked();
}

// Read the /api/ds/query response body inside the waitForQueryDataResponse predicate
// to avoid CDP buffer eviction after the promise resolves.
// TODO: remove once @grafana/plugin-e2e exposes body reading natively.
async function waitForQueryDataBody(explorePage: {
  waitForQueryDataResponse: (
    cb?: (r: { ok(): boolean; json(): Promise<unknown> }) => boolean | Promise<boolean>
  ) => Promise<unknown>;
}): Promise<{ responsePromise: Promise<unknown>; getBody: () => unknown }> {
  let body: unknown = null;
  const responsePromise = explorePage.waitForQueryDataResponse(async (r) => {
    if (!r.ok()) {
      return false;
    }
    const b = await r.json().catch(() => null);
    if (!Array.isArray((b as any)?.results?.A?.frames)) {
      return false;
    }
    body = b;
    return true;
  });
  return { responsePromise, getBody: () => body };
}

test.describe('Query editor', () => {
  test.describe('rendering', () => {
    test(
      'smoke: renders all editor mode options',
      { tag: '@plugins' },
      async ({ page, readProvisionedDataSource }) => {
        const ds = await readProvisionedDataSource({ fileName: PROVISIONED_FILE });
        await page.goto(exploreUrl(ds.uid));
        await expect(page.getByRole('radio', { name: 'Builder', exact: true })).toBeVisible();
        await expect(page.getByRole('radio', { name: 'Code', exact: true })).toBeVisible();
      }
    );

    test('renders format dropdown in all modes', async ({ page, explorePage, readProvisionedDataSource }) => {
      const ds = await readProvisionedDataSource({ fileName: PROVISIONED_FILE });
      await page.goto(exploreUrl(ds.uid));
      const formatSelect = explorePage.getQueryEditorRow('A').getByRole('combobox', { name: 'Format' });
      await switchEditorMode(page, 'Builder');
      await expect(formatSelect).toBeVisible();
      await switchEditorMode(page, 'Code');
      await expect(formatSelect).toBeVisible();
    });

    test('renders run query button in all modes', async ({ page, explorePage, readProvisionedDataSource }) => {
      const ds = await readProvisionedDataSource({ fileName: PROVISIONED_FILE });
      await page.goto(exploreUrl(ds.uid));
      const runQueryButton = explorePage.getQueryEditorRow('A').getByRole('button', { name: 'Run query', exact: true });
      await switchEditorMode(page, 'Builder');
      await expect(runQueryButton).toBeVisible();
      await switchEditorMode(page, 'Code');
      await expect(runQueryButton).toBeVisible();
    });
  });

  test.describe('builder mode', () => {
    test('shows expected fields', async ({ page, readProvisionedDataSource }) => {
      const ds = await readProvisionedDataSource({ fileName: PROVISIONED_FILE });
      await page.goto(exploreUrl(ds.uid));
      await switchEditorMode(page, 'Builder');

      await expect(page.getByRole('switch', { name: 'Filter' })).toBeVisible();
      await expect(page.getByRole('switch', { name: 'Group' })).toBeVisible();
      await expect(page.getByRole('switch', { name: 'Order' })).toBeVisible();
      await expect(page.getByRole('switch', { name: 'Preview' })).toBeVisible();

      await expect(page.getByRole('combobox', { name: 'Table' })).toBeVisible();
      await expect(page.getByRole('combobox', { name: 'Data operations' })).toBeVisible();
      await expect(page.getByRole('combobox', { name: 'Column' })).toBeVisible();
      await expect(page.getByRole('combobox', { name: 'Alias' })).toBeVisible();
    });

    test('can select a dataset, table, and column', async ({ page, explorePage, readProvisionedDataSource }) => {
      const ds = await readProvisionedDataSource({ fileName: PROVISIONED_FILE });
      await page.goto(exploreUrl(ds.uid));
      await switchEditorMode(page, 'Builder');

      const queryRow = explorePage.getQueryEditorRow('A');

      await page.getByRole('combobox', { name: 'Table' }).click();
      await page.getByRole('option', { name: 'server_metrics' }).click();
      await expect(queryRow).toContainText('server_metrics');

      await page.getByRole('combobox', { name: 'Column' }).click();
      await page.getByRole('option', { name: 'app' }).click();
      await expect(queryRow).toContainText('app');
    });
  });

  test.describe('code mode', () => {
    test('shows expected fields', async ({ page, readProvisionedDataSource }) => {
      const ds = await readProvisionedDataSource({ fileName: PROVISIONED_FILE });
      await page.goto(exploreUrl(ds.uid));
      await switchEditorMode(page, 'Code');
      await expect(page.getByRole('textbox', { name: /editor content/i })).toBeVisible();
    });

    test('can enter a SQL query string', async ({ page, readProvisionedDataSource }) => {
      const ds = await readProvisionedDataSource({ fileName: PROVISIONED_FILE });
      await page.goto(exploreUrl(ds.uid));
      await switchEditorMode(page, 'Code');
      const textbox = page.getByRole('textbox', { name: /editor content/i });
      const sql = 'SELECT * FROM server_metrics LIMIT 5';
      await textbox.fill(sql);
      await expect(textbox).toHaveValue(sql);
    });
  });
});

test.describe('Query editor with dynamic data', () => {
  test.describe.configure({ mode: 'serial' });

  test('query returns results with expected columns', async ({ page, explorePage, readProvisionedDataSource }) => {
    const ds = await readProvisionedDataSource({ fileName: PROVISIONED_FILE });
    const sql = 'SELECT * FROM server_metrics LIMIT 1';
    let body: Record<string, unknown> | null = null;
    const responsePromise = explorePage.waitForQueryDataResponse(async (r) => {
      if (!r.ok()) {
        return false;
      }
      const b: any = await r.json().catch(() => null);
      if (!Array.isArray(b?.results?.A?.frames)) {
        return false;
      }
      body = b;
      return true;
    });
    await page.goto(exploreUrl(ds.uid, { rawSql: sql }));
    await responsePromise;
    expect((body as any)?.results?.A?.frames?.length).toBe(1);
    const fields: string[] = (body as any)?.results?.A?.frames?.[0]?.schema?.fields?.map(
      (f: { name: string }) => f.name
    );
    expect(fields).toContain('app');
    expect(fields).toContain('datacenter');
    expect(fields).toContain('host');
    expect(fields).toContain('host_dns');
    expect(fields).toContain('time');
    expect(fields).toContain('usage');
    expect(fields).toContain('usage_bytes');
    expect(fields).toContain('utilisation');
  });

  test('query with basic filter returns results', async ({ page, explorePage, readProvisionedDataSource }) => {
    const ds = await readProvisionedDataSource({ fileName: PROVISIONED_FILE });
    const sql = "SELECT * FROM server_metrics WHERE datacenter = 'afr0' LIMIT 5";
    let body: Record<string, unknown> | null = null;
    const responsePromise = explorePage.waitForQueryDataResponse(async (r) => {
      if (!r.ok()) {
        return false;
      }
      const b: any = await r.json().catch(() => null);
      if (!Array.isArray(b?.results?.A?.frames)) {
        return false;
      }
      body = b;
      return true;
    });
    await page.goto(exploreUrl(ds.uid, { rawSql: sql }));
    await responsePromise;
    expect((body as any)?.results?.A?.frames?.length).toBeGreaterThan(0);
    expect((body as any)?.results?.A?.frames?.length).toBeLessThanOrEqual(5);
  });

  test('query with macro filter returns results', async ({ page, explorePage, readProvisionedDataSource }) => {
    const ds = await readProvisionedDataSource({ fileName: PROVISIONED_FILE });
    const sql = "SELECT TOP 5 * FROM dbo.WORLD_DATA WHERE $__timeFilter(DATE_TIME)";
    let body: Record<string, unknown> | null = null;
    const responsePromise = explorePage.waitForQueryDataResponse(async (r) => {
      if (!r.ok()) {
        return false;
      }
      const b: any = await r.json().catch(() => null);
      if (!Array.isArray(b?.results?.A?.frames)) {
        return false;
      }
      body = b;
      return true;
    });
    await page.goto(exploreUrl(ds.uid, { rawSql: sql }));
    await responsePromise;
    expect((body as any)?.results?.A?.frames?.length).toBeGreaterThan(0);
    expect((body as any)?.results?.A?.frames?.length).toBeLessThanOrEqual(5);
  });
});
