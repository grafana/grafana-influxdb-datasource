import { expect, test } from '@grafana/plugin-e2e';

import { InfluxOptions } from '../../src/types';

const PLUGIN_TYPE = 'influxdb';
const PROVISIONED_FILE = 'datasources.yml';

const DS_URL = process.env.DS_INSTANCE_URL ?? 'http://influxdb:8181';
const DS_VERSION = process.env.DS_INSTANCE_VERSION ?? 'SQL';
const DS_DB_NAME = process.env.DS_INSTANCE_DB_NAME ?? 'grafanainfluxdb';
const DS_INSECURE_GRPC = process.env.DS_INSTANCE_INSECURE_GRPC !== 'false';

test.describe('Config editor', () => {
  test.describe('rendering', () => {
    test('smoke: renders config editor', { tag: '@plugins' }, async ({
      createDataSourceConfigPage,
      page
    }) => {
      await createDataSourceConfigPage({ type: PLUGIN_TYPE });
      await expect(page.getByText(/^Type\s*InfluxDB$/, { exact: true })).toBeVisible();
      await expect(page.getByRole('heading', { name: 'URL and authentication', exact: true })).toBeVisible();
    });

    test('should render URL and authentication section', async ({
      createDataSourceConfigPage,
      page
    }) => {
      await createDataSourceConfigPage({ type: PLUGIN_TYPE });
      const section = page.getByRole('heading', { name: 'URL and authentication', exact: true });
      await section.scrollIntoViewIfNeeded();
      await expect(page.getByTestId('influxdb-v2-config-url-input')).toBeVisible();
      await expect(page.getByTestId('influxdb-v2-config-product-select')).toBeVisible();
      await expect(page.getByTestId('influxdb-v2-config-query-language-select')).toBeVisible();
      await expect(page.getByTestId('influxdb-v2-config-advanced-http-settings-toggle')).toBeVisible();
      await expect(page.getByTestId('influxdb-v2-config-auth-settings-toggle')).toBeVisible();
    });

    test('should render database settings section', async ({
      createDataSourceConfigPage,
      page
    }) => {
      await createDataSourceConfigPage({ type: PLUGIN_TYPE });
      const section = page.getByRole('heading', { name: 'Database settings', exact: true });
      await section.scrollIntoViewIfNeeded();
      await expect(section).toBeVisible();
      await expect(page.getByText('Query language required')).toBeVisible();
    });
  });

  test.describe('provisioned datasource', () => {
    test('should load provisioned URL and authentication settings', async ({
      readProvisionedDataSource,
      gotoDataSourceConfigPage,
      page
    }) => {
      const ds = await readProvisionedDataSource<InfluxOptions>({ fileName: PROVISIONED_FILE });
      await gotoDataSourceConfigPage(ds.uid);
      await expect(page.getByTestId('influxdb-v2-config-url-input')).toHaveValue(DS_URL);
      await expect(page.getByTestId('influxdb-v2-config-query-language-select')).toHaveValue(DS_VERSION);
    });

    test('should load provisioned database settings', async ({
      readProvisionedDataSource,
      gotoDataSourceConfigPage,
      page
    }) => {
      const ds = await readProvisionedDataSource<InfluxOptions>({ fileName: PROVISIONED_FILE });
      await gotoDataSourceConfigPage(ds.uid);
      await expect(page.getByRole('textbox', { name: 'Database' })).toHaveValue(DS_DB_NAME);
      const insecureCheckbox = page.getByRole('checkbox', { name: 'Insecure Connection', exact: true });
      await (DS_INSECURE_GRPC ? expect(insecureCheckbox).toBeChecked() : expect(insecureCheckbox).not.toBeChecked());
    });
  });

  test.describe('save & test', () => {
    test('should pass health check for provisioned datasource', async ({
      readProvisionedDataSource,
      gotoDataSourceConfigPage,
      page
    }) => {
      const ds = await readProvisionedDataSource<InfluxOptions>({ fileName: PROVISIONED_FILE });
      const configPage = await gotoDataSourceConfigPage(ds.uid);
      await page.getByRole('button', { name: /^(Save & test|Test)$/ }).click();
      await expect(configPage).toHaveAlert('success');
    });

    test('should show error alert when health check fails', async ({
      createDataSourceConfigPage,
      page
    }) => {
      const configPage = await createDataSourceConfigPage({ type: PLUGIN_TYPE });
      await page.getByTestId('influxdb-v2-config-url-input').fill('http://influxdb:8181');
      await configPage.mockHealthCheckResponse({ message: 'health check failed' }, 400);
      await page.getByRole('button', { name: /^(Save & test|Test)$/ }).click();
      await expect(configPage).toHaveAlert('error');
    });

    test('should show error alert when backend is unreachable', async ({
      createDataSourceConfigPage,
      page
    }) => {
      const configPage = await createDataSourceConfigPage({ type: PLUGIN_TYPE });
      await page.getByTestId('influxdb-v2-config-url-input').fill('http://localhost:8182');
      await page.getByRole('button', { name: /^(Save & test|Test)$/ }).click();
      await expect(configPage).toHaveAlert('error');
    });
  });
});
