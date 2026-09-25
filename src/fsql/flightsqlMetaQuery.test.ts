import { buildAllTablesQuery, buildColumnQuery, buildTableQuery } from './flightsqlMetaQuery';

describe('buildTableQuery', () => {
  it('filters on the given schema', () => {
    expect(buildTableQuery('iox')).toEqual(
      "SELECT table_name FROM information_schema.tables WHERE table_schema = 'iox' ORDER BY table_name"
    );
  });
});

describe('buildAllTablesQuery', () => {
  it('lists tables from every user-facing schema', () => {
    expect(buildAllTablesQuery()).toEqual(
      "SELECT table_schema, table_name FROM information_schema.tables WHERE table_schema != 'information_schema' ORDER BY table_schema, table_name"
    );
  });
});

describe('buildColumnQuery', () => {
  it('matches the full table name even when it contains dots', () => {
    expect(buildColumnQuery('Windows.PerfCounters.Memory', 'iox')).toEqual(
      "SELECT column_name, data_type FROM information_schema.columns WHERE table_schema = 'iox' AND table_name = 'Windows.PerfCounters.Memory' ORDER BY column_name"
    );
  });

  it('unquotes a table value saved with quotes', () => {
    expect(buildColumnQuery('"Windows.PerfCounters.Memory"', 'iox')).toEqual(
      "SELECT column_name, data_type FROM information_schema.columns WHERE table_schema = 'iox' AND table_name = 'Windows.PerfCounters.Memory' ORDER BY column_name"
    );
  });

  it('splits a schema-qualified quoted table into schema and table', () => {
    expect(buildColumnQuery('"system"."queries"', 'iox')).toEqual(
      "SELECT column_name, data_type FROM information_schema.columns WHERE table_schema = 'system' AND table_name = 'queries' ORDER BY column_name"
    );
  });

  it('uses the given schema for a plain table name', () => {
    expect(buildColumnQuery('cpu', 'iox')).toEqual(
      "SELECT column_name, data_type FROM information_schema.columns WHERE table_schema = 'iox' AND table_name = 'cpu' ORDER BY column_name"
    );
  });
});
