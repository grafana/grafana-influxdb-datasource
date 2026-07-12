import { QueryEditorExpressionType, type SQLQuery } from '@grafana/sql';

import { toRawSql } from './sqlUtil';

describe('toRawSql', () => {
  it('should render sql properly', () => {
    const expected = 'SELECT "host" FROM "value1" WHERE "time" >= $__timeFrom AND "time" <= $__timeTo LIMIT 50';
    const testQuery: SQLQuery = {
      refId: 'A',
      sql: {
        limit: 50,
        columns: [
          {
            parameters: [
              {
                name: 'host',
                type: QueryEditorExpressionType.FunctionParameter,
              },
            ],
            type: QueryEditorExpressionType.Function,
          },
        ],
      },
      dataset: 'iox',
      table: 'value1',
    };
    const result = toRawSql(testQuery);
    expect(result).toEqual(expected);
  });

  it('should wrap the identifiers with quote', () => {
    const expected = 'SELECT "host" FROM "TestValue" WHERE "time" >= $__timeFrom AND "time" <= $__timeTo LIMIT 50';
    const testQuery: SQLQuery = {
      refId: 'A',
      sql: {
        limit: 50,
        columns: [
          {
            parameters: [
              {
                name: 'host',
                type: QueryEditorExpressionType.FunctionParameter,
              },
            ],
            type: QueryEditorExpressionType.Function,
          },
        ],
      },
      dataset: 'iox',
      table: 'TestValue',
    };
    const result = toRawSql(testQuery);
    expect(result).toEqual(expected);
  });

  it('should wrap filters in where', () => {
    const expected = `SELECT "host" FROM "TestValue" WHERE "time" >= $__timeFrom AND "time" <= $__timeTo AND ("sensor_id" = '12' AND "sensor_id" = '23') LIMIT 50`;
    const testQuery: SQLQuery = {
      refId: 'A',
      sql: {
        limit: 50,
        columns: [
          {
            parameters: [
              {
                name: 'host',
                type: QueryEditorExpressionType.FunctionParameter,
              },
            ],
            type: QueryEditorExpressionType.Function,
          },
        ],
        whereString: `(sensor_id = '12' AND sensor_id = '23')`,
      },
      dataset: 'iox',
      table: 'TestValue',
    };
    const result = toRawSql(testQuery);
    expect(result).toEqual(expected);
  });

  it('should quote a table name containing dots exactly once', () => {
    const expected =
      'SELECT "host" FROM "Windows.PerfCounters.Memory" WHERE "time" >= $__timeFrom AND "time" <= $__timeTo LIMIT 50';
    const testQuery: SQLQuery = {
      refId: 'A',
      sql: {
        limit: 50,
        columns: [
          {
            parameters: [{ name: 'host', type: QueryEditorExpressionType.FunctionParameter }],
            type: QueryEditorExpressionType.Function,
          },
        ],
      },
      dataset: 'iox',
      table: 'Windows.PerfCounters.Memory',
    };
    expect(toRawSql(testQuery)).toEqual(expected);
  });

  it('should not double-quote a table value saved with quotes', () => {
    const expected =
      'SELECT "host" FROM "Windows.PerfCounters.Memory" WHERE "time" >= $__timeFrom AND "time" <= $__timeTo LIMIT 50';
    const testQuery: SQLQuery = {
      refId: 'A',
      sql: {
        limit: 50,
        columns: [
          {
            parameters: [{ name: 'host', type: QueryEditorExpressionType.FunctionParameter }],
            type: QueryEditorExpressionType.Function,
          },
        ],
      },
      dataset: 'iox',
      table: '"Windows.PerfCounters.Memory"',
    };
    expect(toRawSql(testQuery)).toEqual(expected);
  });

  it('should pass a schema-qualified quoted table through unchanged, without a time filter', () => {
    const expected = 'SELECT "duration" FROM "system"."queries" LIMIT 50';
    const testQuery: SQLQuery = {
      refId: 'A',
      sql: {
        limit: 50,
        columns: [
          {
            parameters: [{ name: 'duration', type: QueryEditorExpressionType.FunctionParameter }],
            type: QueryEditorExpressionType.Function,
          },
        ],
      },
      dataset: 'iox',
      table: '"system"."queries"',
    };
    expect(toRawSql(testQuery)).toEqual(expected);
  });

  it('should not double-quote a column name saved with quotes', () => {
    const expected = 'SELECT "my/field" FROM "cpu" WHERE "time" >= $__timeFrom AND "time" <= $__timeTo LIMIT 50';
    const testQuery: SQLQuery = {
      refId: 'A',
      sql: {
        limit: 50,
        columns: [
          {
            parameters: [{ name: '"my/field"', type: QueryEditorExpressionType.FunctionParameter }],
            type: QueryEditorExpressionType.Function,
          },
        ],
      },
      dataset: 'iox',
      table: 'cpu',
    };
    expect(toRawSql(testQuery)).toEqual(expected);
  });

  it('should not wrap * with quote', () => {
    const expected = 'SELECT * FROM "TestValue" WHERE "time" >= $__timeFrom AND "time" <= $__timeTo LIMIT 50';
    const testQuery: SQLQuery = {
      refId: 'A',
      sql: {
        limit: 50,
        columns: [
          {
            parameters: [
              {
                name: '*',
                type: QueryEditorExpressionType.FunctionParameter,
              },
            ],
            type: QueryEditorExpressionType.Function,
          },
        ],
      },
      dataset: 'iox',
      table: 'TestValue',
    };
    const result = toRawSql(testQuery);
    expect(result).toEqual(expected);
  });
});
