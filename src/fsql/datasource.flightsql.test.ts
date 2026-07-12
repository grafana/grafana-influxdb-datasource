import { type TemplateSrv } from '@grafana/runtime';

import { getMockDSInstanceSettings, mockBackendService } from '../mocks/datasource';
import { mockInfluxSQLVariableFetchResponse } from '../mocks/response';

import { FlightSQLDatasource } from './datasource.flightsql';

mockBackendService(mockInfluxSQLVariableFetchResponse);
describe('flightsql datasource', () => {
  const templateSrv: TemplateSrv = {
    containsTemplate: jest.fn(),
    replace: jest.fn().mockImplementation((val: string) => val),
    updateTimeRange: jest.fn(),
    getVariables: jest.fn().mockReturnValue([
      {
        name: 'templateVar',
        text: 'templateVar',
        value: 'templateVar',
        type: '',
        label: 'templateVar',
      },
    ]),
  };
  const mockInstanceSettings = getMockDSInstanceSettings();
  const instanceSettings = {
    ...mockInstanceSettings,
    jsonData: {
      ...mockInstanceSettings.jsonData,
      allowCleartextPasswords: false,
      tlsAuth: false,
      tlsAuthWithCACert: false,
      tlsSkipVerify: false,
      maxIdleConns: 1,
      maxOpenConns: 1,
      maxIdleConnsAuto: true,
      connMaxLifetime: 1,
      timezone: '',
      user: '',
      database: '',
      url: '',
      timeInterval: '',
    },
  };
  const ds = new FlightSQLDatasource(instanceSettings, templateSrv);

  it('should add template variables to the responses', async () => {
    const fields = await ds.fetchFields({ dataset: 'test', table: 'table' });
    expect(fields[0].name).toBe('$templateVar');
  });

  it('should list schemas from information_schema instead of hardcoding iox', async () => {
    const runSql = jest.spyOn(ds, 'runSql').mockResolvedValue([['iox'], ['system']] as never);
    const datasets = await ds.fetchDatasets();
    expect(runSql.mock.calls[0][0]).toContain('SELECT DISTINCT table_schema FROM information_schema.tables');
    expect(datasets).toEqual(['iox', 'system']);
    runSql.mockRestore();
  });

  it('should return raw table names without pre-quoting', async () => {
    const runSql = jest
      .spyOn(ds, 'runSql')
      .mockResolvedValue([['Windows.PerfCounters.Memory'], ['cpu']] as never);
    const tables = await ds.fetchTables('iox');
    expect(tables).toEqual(['$templateVar', 'Windows.PerfCounters.Memory', 'cpu']);
    runSql.mockRestore();
  });

  it('should qualify tables outside the default iox schema', async () => {
    const runSql = jest.spyOn(ds, 'runSql').mockResolvedValue([
      ['iox', 'Windows.PerfCounters.Memory'],
      ['iox', 'cpu'],
      ['system', 'queries'],
    ] as never);
    const tables = await ds.fetchAllTables();
    expect(tables).toEqual(['$templateVar', 'Windows.PerfCounters.Memory', 'cpu', '"system"."queries"']);
    runSql.mockRestore();
  });

  it('should return raw field values without pre-quoting', async () => {
    const runSql = jest.spyOn(ds, 'runSql').mockResolvedValue([['my/field', 'FLOAT64']] as never);
    const fields = await ds.fetchFields({ dataset: 'iox', table: 'cpu' });
    expect(fields.map((f) => f.value)).toEqual(['$templateVar', 'my/field']);
    runSql.mockRestore();
  });
});
