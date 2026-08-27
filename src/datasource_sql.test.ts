import { lastValueFrom } from 'rxjs';

import { type SQLQuery } from '@grafana/sql';

import { FSQLEditor } from './components/editor/lazyEditors';
import InfluxDatasource from './datasource';
import { getMockDSInstanceSettings, mockBackendService, mockTemplateSrv } from './mocks/datasource';
import { mockInfluxQueryRequest } from './mocks/request';
import { mockInfluxSQLFetchResponse } from './mocks/response';
import { InfluxVersion } from './types';

mockBackendService(mockInfluxSQLFetchResponse);

describe('InfluxDB SQL Support', () => {
  const replaceMock = jest.fn();
  const templateSrv = mockTemplateSrv(replaceMock);

  let sqlQuery: SQLQuery;

  beforeEach(() => {
    sqlQuery = {
      refId: 'x',
      rawSql:
        'SELECT "$interpolationVar2", time FROM iox.$interpolationVar WHERE time >= $__timeFrom AND time <= $__timeTo',
    };
  });

  describe('interpolate variables', () => {
    const ds = new InfluxDatasource(getMockDSInstanceSettings({ version: InfluxVersion.SQL }), templateSrv);

    it('should call replace template variables for rawSql', async () => {
      await lastValueFrom(ds.query(mockInfluxQueryRequest([sqlQuery])));
      expect(replaceMock.mock.calls[1][0]).toBe(
        `SELECT "$interpolationVar2", time FROM iox.$interpolationVar WHERE time >= $__timeFrom AND time <= $__timeTo`
      );
    });
  });

  describe('annotations', () => {
    const ds = new InfluxDatasource(getMockDSInstanceSettings({ version: InfluxVersion.SQL }), templateSrv);

    it('should use the SQL query editor with standard annotation support', () => {
      expect(ds.annotations?.QueryEditor).toBe(FSQLEditor);
      expect(ds.annotations?.prepareAnnotation).toBeUndefined();
    });
  });

  describe('filterQuery', () => {
    const ds = new InfluxDatasource(getMockDSInstanceSettings({ version: InfluxVersion.SQL }), templateSrv);

    it('should run queries with rawSql', () => {
      expect(ds.filterQuery(sqlQuery)).toBe(true);
    });

    it('should skip queries without rawSql', () => {
      expect(ds.filterQuery({ refId: 'x' })).toBe(false);
      expect(ds.filterQuery({ refId: 'x', rawSql: '' })).toBe(false);
    });
  });
});
