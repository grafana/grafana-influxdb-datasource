import { lastValueFrom } from 'rxjs';

import { type DataQueryRequest, dateTime } from '@grafana/data';
import { type TemplateSrv } from '@grafana/runtime';

import { getMockDSInstanceSettings, getMockInfluxDS } from './mocks/datasource';
import { queryBuilder } from './test/helpers/queryVariableBuilder';
import { type InfluxVariableQuery } from './types';
import { InfluxVariableSupport } from './variables';

describe('InfluxVariableSupport', () => {
  // Chained variable queries must interpolate with the query text as context
  // so values used inside `=~ /^$var$/` get regex-escaped
  it('escapes regex-significant characters in chained variable values', async () => {
    const variableQuery = 'SHOW TAG VALUES WITH KEY = "service" WHERE hostname =~ /^$hostname$/';
    const hostnameVar = queryBuilder().withId('hostname').withName('hostname').withMulti(false).build();
    const templateSrvStub = {
      replace: jest.fn(
        (target: string, _scopedVars: unknown, format: (value: string, variable: unknown, def: unknown) => string) =>
          target.replace(/\$hostname/g, () => String(format('BÖRD 01 (test)', hostnameVar, jest.fn())))
      ),
    } as unknown as TemplateSrv;

    const datasource = getMockInfluxDS(getMockDSInstanceSettings(), templateSrvStub);
    const metricFindQueryMock = jest.fn().mockResolvedValue([]);
    datasource.metricFindQuery = metricFindQueryMock;

    const variableSupport = new InfluxVariableSupport(datasource, templateSrvStub);
    const request = {
      targets: [{ refId: 'A', query: variableQuery }],
      scopedVars: {},
      range: {
        from: dateTime('2026-01-01T00:00:00Z'),
        to: dateTime('2026-01-01T01:00:00Z'),
        raw: { from: 'now-1h', to: 'now' },
      },
      timezone: 'utc',
    } as unknown as DataQueryRequest<InfluxVariableQuery>;

    await lastValueFrom(variableSupport.query(request));

    expect(metricFindQueryMock).toHaveBeenCalledWith(
      expect.objectContaining({
        query: 'SHOW TAG VALUES WITH KEY = "service" WHERE hostname =~ /^BÖRD 01 \\(test\\)$/',
      }),
      expect.anything()
    );
  });
});
