import { lazy, Suspense } from 'react';
import { DataSourcePlugin } from '@grafana/data';
import { config } from '@grafana/runtime';

import InfluxDatasource from './datasource';
import type { Props as ConfigEditorV1Props } from './components/editor/config/ConfigEditor';
import type { Props as ConfigEditorV2Props } from './components/editor/config-v2/types';
import type { Props as LazyQueryEditorProps } from './components/editor/query/QueryEditor';

const LazyConfigEditorV1 = lazy(async () => import('./components/editor/config/ConfigEditor'));
const LazyConfigEditorV2 = lazy(async () => import('./components/editor/config-v2/ConfigEditor'));
const LazyQueryEditor = lazy(async () => import('./components/editor/query/QueryEditor'));
const LazyInfluxStartPage = lazy(async () => import('./components/editor/query/influxql/InfluxStartPage'));

function ConfigEditorV1(props: ConfigEditorV1Props) {
  return (
    <Suspense>
      <LazyConfigEditorV1 {...props} />
    </Suspense>
  );
}

function ConfigEditorV2(props: ConfigEditorV2Props) {
  return (
    <Suspense>
      <LazyConfigEditorV2 {...props} />
    </Suspense>
  );
}

function QueryEditor(props: LazyQueryEditorProps) {
  return (
    <Suspense>
      <LazyQueryEditor {...props} />
    </Suspense>
  );
}

function InfluxStartPage() {
  return (
    <Suspense>
      <LazyInfluxStartPage />
    </Suspense>
  );
}

// ConfigEditorV2 is the new design for the InfluxDB configuration page
const configEditor = config.featureToggles.newInfluxDSConfigPageDesign ? ConfigEditorV2 : ConfigEditorV1;

export const plugin = new DataSourcePlugin(InfluxDatasource)
  .setConfigEditor(configEditor)
  .setQueryEditor(QueryEditor)
  .setQueryEditorHelp(InfluxStartPage);
