import { test, expect } from '@grafana/plugin-e2e';

import { type InfluxOptions } from '../../src/types';

test.describe('Flux query editor', () => {
  test('toggles a line comment with the Flux // marker', async ({ panelEditPage, readProvisionedDataSource, page }) => {
    const ds = await readProvisionedDataSource<InfluxOptions>({
      fileName: 'datasources.yml',
      name: 'InfluxDB v2 (Flux)',
    });
    await panelEditPage.datasource.set(ds.name);

    const editor = panelEditPage.getQueryEditorRow('A').getByRole('textbox', { name: /editor content/i });
    await editor.click();
    await page.keyboard.type('buckets()');
    // The Desktop Chrome device reports a Windows user agent, so Monaco binds the toggle to Ctrl on every host.
    await page.keyboard.press('Control+Slash');

    await expect(editor).toHaveValue('// buckets()');
  });
});
