import { parseQualifiedTable, quoteLiteral, unquoteIdentifier } from './sqlUtil';

export function buildTableQuery(dataset?: string) {
  const database = dataset !== undefined ? quoteIdentAsLiteral(dataset) : 'database()';
  return `SELECT table_name FROM information_schema.tables WHERE table_schema = ${database} ORDER BY table_name`;
}

export function buildSchemaQuery() {
  return `SELECT DISTINCT table_schema FROM information_schema.tables WHERE table_schema != 'information_schema' ORDER BY table_schema`;
}

// Lists tables from every user-facing schema so system tables are reachable
// from the query builder.
export function buildAllTablesQuery() {
  return `SELECT table_schema, table_name FROM information_schema.tables WHERE table_schema != 'information_schema' ORDER BY table_schema, table_name`;
}

export function buildColumnQuery(table: string, dbName?: string) {
  let query = 'SELECT column_name, data_type FROM information_schema.columns WHERE ';
  query += buildTableConstraint(table, dbName);

  query += ' ORDER BY column_name';

  return query;
}

function buildTableConstraint(table: string, dbName?: string) {
  const qualified = parseQualifiedTable(table);
  if (qualified) {
    return `table_schema = ${quoteLiteral(qualified.schema)} AND table_name = ${quoteLiteral(qualified.table)}`;
  }

  // Table names may contain dots, so the full name is matched as-is rather
  // than split into schema and table parts.
  const database = dbName !== undefined ? quoteIdentAsLiteral(dbName) : 'database()';
  return `table_schema = ${database} AND table_name = ` + quoteIdentAsLiteral(table);
}

function quoteIdentAsLiteral(value: string) {
  return quoteLiteral(unquoteIdentifier(value));
}
