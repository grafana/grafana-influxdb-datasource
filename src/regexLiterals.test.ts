import { isVariableInRegexLiteral } from './regexLiterals';

describe('isVariableInRegexLiteral', () => {
  describe('InfluxQL', () => {
    it.each([
      ['after =~', 'host', 'SELECT * FROM "m" WHERE "host" =~ /^$host$/'],
      ['after !~', 'host', 'SELECT * FROM "m" WHERE "host" !~ /^$host$/'],
      ['as a SELECT field', 'f', 'SELECT /^$f$/ FROM "h2o_feet" LIMIT 1'],
      ['as a function argument', 'f', 'SELECT mean(/^$f$/) FROM "h2o_feet"'],
      ['as a GROUP BY dimension', 'tk', 'SELECT mean("v") FROM "m" GROUP BY /^$tk$/'],
      ['as a FROM source', 'm', 'SELECT mean("value") FROM /^$m$/ WHERE $timeFilter'],
      ['as a FROM source after a quoted retention policy', 'm', 'SELECT mean("v") FROM "autogen"./^$m$/'],
      ['as a FROM source after an unquoted retention policy', 'm', 'SELECT mean("v") FROM autogen./^$m$/'],
      ['as a FROM source after database and retention policy', 'm', 'SELECT mean("v") FROM "mydb"."autogen"./^$m$/'],
      ['as the second FROM source', 'm2', 'SELECT mean("v") FROM /^$m1$/, /^$m2$/'],
      ['in a metadata query', 'hostname', 'SHOW TAG VALUES WITH KEY = "service" WHERE hostname =~ /^$hostname$/'],
      ['with the ${var} syntax', 'host', 'SELECT * FROM "m" WHERE "host" =~ /^${host}$/'],
      ['with the ${var:format} syntax', 'host', 'SELECT * FROM "m" WHERE "host" =~ /^${host:regex}$/'],
      ['when the regex contains an escaped slash', 'path', 'SELECT * FROM "m" WHERE "path" =~ /^\\/var\\/$path$/'],
      [
        'when the query also contains a division',
        'mac',
        'SELECT last("sent") / $__interval_ms FROM "m" WHERE "mac" =~ /^$mac$/',
      ],
      [
        'after a string literal containing a slash',
        'host',
        `SELECT * FROM "m" WHERE "cmd" = 'load from /data' AND "host" =~ /^$host$/`,
      ],
      [
        'on a later line of a multi-line query',
        'host',
        'SELECT mean("v") FROM "cpu"\nWHERE "host" =~ /^$host$/\nGROUP BY time(1m)',
      ],
      ['as a lone query builder tag value', 'HPU', '/^$HPU$/'],
      ['after a quoted identifier containing a slash', 'h', 'SELECT "a/b" FROM "m" WHERE "h" =~ /^$h$/'],
      [
        'after dividing a field named like a Flux keyword',
        'sym',
        `SELECT return / 100 FROM "stocks" WHERE "sym" =~ /^$sym$/`,
      ],
    ])('detects a variable used in a regex %s', (_case, name, query) => {
      expect(isVariableInRegexLiteral(name, query)).toBe(true);
    });

    it.each([
      ['in a division', '__interval_ms', 'SELECT last("sent") / $__interval_ms FROM "m" WHERE "mac" =~ /^$mac$/'],
      ['between slashes that follow an identifier', 'tempVar', 'select atan(z/sqrt(3.14)), that where path /$tempVar/'],
      ['in a string literal', 'host', `SELECT * FROM "m" WHERE "host" = '$host' AND "cpu" =~ /^cpu-total$/`],
      ['in a string literal with slashes', 'path', `SELECT * FROM "m" WHERE "path" = '/var/$path/'`],
      ['in a line comment', 'host', `SELECT * FROM "m" WHERE "host" = '$host' -- see /docs/$host/`],
      [
        'after a leading block comment',
        'host',
        `/* panel note */\nSELECT mean("usage_idle") FROM "cpu"\nWHERE "host" = '$host'\nAND "cpu" =~ /^cpu-total$/`,
      ],
      ['when only another variable is in the regex', 'host', 'SELECT * FROM "m" WHERE "host" =~ /^$other$/'],
      ['when a longer variable name shares the prefix', 'host', 'SELECT * FROM "m" WHERE "host" =~ /^$host_name$/'],
      ['as a bare query builder tag value', 'HPU', '$HPU'],
      ['in a division after a quoted identifier containing a slash', 'h', 'SELECT "bytes/sec" / $h FROM "m"'],
      [
        'in a string after dividing a field named like a Flux keyword',
        'sym',
        `SELECT return / 100 FROM "stocks" WHERE "sym" = '$sym'`,
      ],
      ['on the line after an unterminated regex', 'h', 'WHERE a =~ /^x\nAND "h" = $h'],
    ])('does not flag a variable used %s', (_case, name, query) => {
      expect(isVariableInRegexLiteral(name, query)).toBe(false);
    });
  });

  describe('Flux', () => {
    it.each([
      ['in a filter', 'm', 'from(bucket: "b") |> filter(fn: (r) => r._measurement =~ /^${m}$/)'],
      ['with a bracketed record field', 'host', 'from(bucket: "b") |> filter(fn: (r) => r["host"] =~ /^${host}$/)'],
      [
        'after a line comment',
        'host',
        '// filter by host\nfrom(bucket: "b")\n  |> filter(fn: (r) => r.host =~ /^${host}$/)',
      ],
      [
        'after dividing a member named like a keyword',
        'x',
        'from(bucket: "b") |> map(fn: (r) => ({ r with v: r.from / 2 })) |> filter(fn: (r) => r.h =~ /^${x}$/)',
      ],
    ])('detects a variable used in a regex %s', (_case, name, query) => {
      expect(isVariableInRegexLiteral(name, query)).toBe(true);
    });

    it.each([
      ['in a string literal', 'host', 'from(bucket: "b") |> filter(fn: (r) => r.host == "${host}")'],
      [
        'in a line comment',
        'host',
        '// ${host} lives here /tmp/${host}/\nfrom(bucket: "b") |> filter(fn: (r) => r.cpu =~ /^cpu-total$/)',
      ],
      ['in a division', 'divisor', 'from(bucket: "b") |> map(fn: (r) => ({ r with _value: r._value / ${divisor} }))'],
      [
        'in a string after dividing a member named like a keyword',
        'host',
        'from(bucket: "b") |> map(fn: (r) => ({ r with rate: r.from / 60.0, label: "${host}" }))',
      ],
      [
        'in a string after dividing a float with an empty fraction',
        'host',
        'from(bucket: "b") |> map(fn: (r) => ({ r with rate: 0. / r._value, label: "${host}" }))',
      ],
      ['in a division after a ${} reference', 'h', 'x = ${a} / ${h}'],
      ['in a division after an index expression', 'h', 'x = arr[0] / ${h}'],
    ])('does not flag a variable used %s', (_case, name, query) => {
      expect(isVariableInRegexLiteral(name, query)).toBe(false);
    });
  });
});
