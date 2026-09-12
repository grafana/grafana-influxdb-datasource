import { extend, has, isString, omit, pick, reduce } from 'lodash';
import { lastValueFrom, merge, Observable, of, throwError } from 'rxjs';
import { catchError, map } from 'rxjs/operators';

import {
  type AdHocVariableFilter,
  type AnnotationEvent,
  type DataFrame,
  type DataQueryError,
  type DataQueryRequest,
  type DataQueryResponse,
  type DataSourceGetTagKeysOptions,
  type DataSourceGetTagValuesOptions,
  type DataSourceInstanceSettings,
  dateMath,
  type DateTime,
  escapeRegex,
  FieldType,
  type MetricFindValue,
  type QueryVariableModel,
  type RawTimeRange,
  type ScopedVars,
  toDataFrame,
} from '@grafana/data';
import {
  type BackendDataSourceResponse,
  DataSourceWithBackend,
  type FetchResponse,
  getBackendSrv,
  getTemplateSrv,
  type TemplateSrv,
} from '@grafana/runtime';
import { QueryFormat, type SQLQuery } from '@grafana/sql';

import { AnnotationEditor } from './components/editor/annotation/AnnotationEditor';
import { FluxQueryEditor } from './components/editor/query/flux/FluxQueryEditor';
import { FSQLEditor } from './components/editor/query/fsql/FSQLEditor';
import { BROWSER_MODE_DISABLED_MESSAGE } from './constants';
import { toRawSql } from './fsql/sqlUtil';
import InfluxQueryModel from './influx_query_model';
import InfluxSeries from './influx_series';
import { buildMetadataQuery } from './influxql_query_builder';
import { prepareAnnotation } from './migrations';
import { buildRawQuery, removeRegexWrapper } from './queryUtils';
import ResponseParser from './response_parser';
import { DEFAULT_POLICY, type InfluxOptions, type InfluxQuery, type InfluxVariableQuery, InfluxVersion } from './types';
import { InfluxVariableSupport } from './variables';

/**
 * Decides whether a variable is referenced inside a regex literal in the
 * given query text. InfluxQL and Flux regex literals only appear after the
 * `=~` and `!~` operators or in a FROM clause, or the interpolated field is
 * itself a lone regex (a query-builder tag value or measurement such as
 * `/^$var$/`). A `/` anywhere else (e.g. division) does not open a regex.
 */
export function isVariableInRegexLiteral(name: string, query: string): boolean {
  const escapedName = escapeRegex(name);
  // Matches $name and ${name} / ${name:format} references
  const varRef = new RegExp(`\\$(?:${escapedName}\\b|\\{${escapedName}(?::[^}]*)?\\})`);
  const literals: string[] = [];

  const trimmed = query.trim();
  if (trimmed.length > 1 && trimmed.startsWith('/') && trimmed.endsWith('/')) {
    literals.push(trimmed.slice(1, -1));
  }

  // Regex literals anchored by an operator or FROM (with optional retention policy)
  const anchoredLiteral = /(?:=~|!~|\bfrom\b(?:\s*"[^"]*"\s*\.)?)\s*\/((?:[^/\\]|\\.)*)\//gi;
  for (const match of query.matchAll(anchoredLiteral)) {
    literals.push(match[1]);
  }

  return literals.some((literal) => varRef.test(literal));
}

export default class InfluxDatasource extends DataSourceWithBackend<InfluxQuery, InfluxOptions> {
  type: string;
  urls: string[];
  username: string;
  password: string;
  name: string;
  database?: string;
  basicAuth?: string;
  withCredentials?: boolean;
  access: 'direct' | 'proxy';
  responseParser: ResponseParser;
  httpMode: string;
  version?: InfluxVersion;
  isProxyAccess: boolean;
  showTagTime: string;

  constructor(
    instanceSettings: DataSourceInstanceSettings<InfluxOptions>,
    readonly templateSrv: TemplateSrv = getTemplateSrv()
  ) {
    super(instanceSettings);

    this.type = 'influxdb';
    this.urls = (instanceSettings.url ?? '').split(',').map((url) => {
      return url.trim();
    });

    this.username = instanceSettings.username ?? '';
    this.password = instanceSettings.password ?? '';
    this.name = instanceSettings.name;
    this.basicAuth = instanceSettings.basicAuth;
    this.withCredentials = instanceSettings.withCredentials;
    this.access = instanceSettings.access;
    const settingsData: InfluxOptions = instanceSettings.jsonData ?? {};
    this.database = settingsData.dbName ?? instanceSettings.database;
    this.interval = settingsData.timeInterval;
    this.showTagTime = settingsData.showTagTime || '';
    this.httpMode = settingsData.httpMode || 'GET';
    this.responseParser = new ResponseParser();
    this.version = settingsData.version ?? InfluxVersion.InfluxQL;
    this.isProxyAccess = instanceSettings.access === 'proxy';
    this.variables = new InfluxVariableSupport(this, this.templateSrv);

    if (this.version === InfluxVersion.Flux) {
      // When flux, use an annotation processor rather than the `annotationQuery` lifecycle
      this.annotations = {
        QueryEditor: FluxQueryEditor,
      };
    } else if (this.version === InfluxVersion.SQL) {
      // SQL annotations run through the standard annotation pipeline: the
      // regular backend query executes the rawSql and the frame fields are
      // mapped to annotation events by the shared field mapper
      this.annotations = {
        QueryEditor: FSQLEditor,
      };
    } else {
      this.annotations = {
        QueryEditor: AnnotationEditor,
        prepareAnnotation,
      };
    }
  }

  query(request: DataQueryRequest<InfluxQuery>): Observable<DataQueryResponse> {
    if (!this.isProxyAccess) {
      const error = new Error(BROWSER_MODE_DISABLED_MESSAGE);
      return throwError(() => error);
    }

    return this._query(request);
  }

  _query(request: DataQueryRequest<InfluxQuery>): Observable<DataQueryResponse> {
    const filteredRequest = {
      ...request,
      targets: request.targets.filter((t) => t.hide !== true),
    };

    // migrate annotations
    if (filteredRequest.targets.some((target: InfluxQuery) => target.fromAnnotations)) {
      const streams: Array<Observable<DataQueryResponse>> = [];

      for (const target of filteredRequest.targets) {
        if (target.query) {
          streams.push(
            new Observable((subscriber) => {
              this.annotationEvents(filteredRequest, target)
                .then((events) => subscriber.next({ data: [toDataFrame(events)] }))
                .catch((ex) => subscriber.error(new Error(ex)))
                .finally(() => subscriber.complete());
            })
          );
        }
      }

      return merge(...streams);
    }

    return super.query(filteredRequest);
  }

  getQueryDisplayText(query: InfluxQuery) {
    switch (this.version) {
      case InfluxVersion.Flux:
        return query.query;
      case InfluxVersion.SQL:
        return toRawSql(query);
      case InfluxVersion.InfluxQL:
        return new InfluxQueryModel(query).render(false);
      default:
        return '';
    }
  }

  /**
   * Returns false if the query should be skipped
   */
  filterQuery(query: InfluxQuery & SQLQuery): boolean {
    if (this.version === InfluxVersion.Flux) {
      return !!query.query;
    }
    if (this.version === InfluxVersion.SQL) {
      return !!query.rawSql;
    }
    return true;
  }

  applyTemplateVariables(
    query: InfluxQuery,
    scopedVars: ScopedVars,
    filters?: AdHocVariableFilter[]
  ): InfluxQuery & SQLQuery {
    const variables = scopedVars || {};

    // We want to interpolate these variables on backend.
    // The pre-calculated values are replaced with the variable strings.
    variables.__interval = {
      value: '$__interval',
    };
    variables.__interval_ms = {
      value: '$__interval_ms',
    };

    if (this.version === InfluxVersion.Flux) {
      return {
        ...query,
        query: this.templateSrv.replace(query.query ?? '', variables), // The raw query text
      };
    }

    if (this.version === InfluxVersion.SQL || this.access === 'proxy') {
      query = this.applyVariables(query, variables, filters);
      if (query.adhocFilters?.length) {
        query.adhocFilters = (query.adhocFilters ?? []).map((af) => {
          const { condition, ...asTag } = af;
          asTag.value = this.templateSrv.replace(asTag.value ?? '', variables);
          return asTag;
        });
        query.tags = [...(query.tags ?? []), ...query.adhocFilters];
      }
    }

    return query;
  }

  targetContainsTemplate(target: InfluxQuery) {
    // for flux-mode we just take target.query,
    // for influxql-mode we use InfluxQueryModel to create the text-representation
    const queryText = this.version === InfluxVersion.Flux ? target.query : buildRawQuery(target);

    return this.templateSrv.containsTemplate(queryText);
  }

  interpolateVariablesInQueries(
    queries: InfluxQuery[],
    scopedVars: ScopedVars,
    filters?: AdHocVariableFilter[]
  ): InfluxQuery[] {
    if (!queries || queries.length === 0) {
      return [];
    }

    return queries.map((query) => {
      if (this.version === InfluxVersion.Flux) {
        return {
          ...query,
          datasource: this.getRef(),
          query: this.templateSrv.replace(
            query.query ?? '',
            scopedVars,
            (value: string | string[] = [], variable: QueryVariableModel) =>
              this.interpolateQueryExpr(value, variable, query.query)
          ), // The raw query text
        };
      }

      const queryWithVariables = this.applyVariables(query, scopedVars, filters);
      if (queryWithVariables.adhocFilters?.length) {
        queryWithVariables.adhocFilters = (queryWithVariables.adhocFilters ?? []).map((af) => {
          const { condition, ...asTag } = af;
          asTag.value = this.templateSrv.replace(asTag.value ?? '', scopedVars);
          return asTag;
        });
        queryWithVariables.tags = [...(queryWithVariables.tags ?? []), ...queryWithVariables.adhocFilters];
      }

      return {
        ...queryWithVariables,
        datasource: this.getRef(),
      };
    });
  }

  applyVariables(query: InfluxQuery & SQLQuery, scopedVars: ScopedVars, filters?: AdHocVariableFilter[]) {
    const expandedQuery = { ...query };
    if (query.groupBy) {
      expandedQuery.groupBy = query.groupBy.map((groupBy) => {
        return {
          ...groupBy,
          params: groupBy.params?.map((param) => this.templateSrv.replace(param.toString(), undefined)),
        };
      });
    }

    if (query.select) {
      expandedQuery.select = query.select.map((selects) => {
        return selects.map((select) => {
          return {
            ...select,
            params: select.params?.map((param) => this.templateSrv.replace(param.toString(), scopedVars)),
          };
        });
      });
    }

    if (query.tags) {
      expandedQuery.tags = query.tags.map((tag) => {
        // Remove the regex wrapper if the operator is not a regex operator
        if (tag.operator !== '=~' && tag.operator !== '!~') {
          tag.value = removeRegexWrapper(tag.value);
        }

        return {
          ...tag,
          key: this.templateSrv.replace(tag.key, scopedVars),
          value: this.templateSrv.replace(
            tag.value ?? '',
            scopedVars,
            (value: string | string[] = [], variable: QueryVariableModel) =>
              this.interpolateQueryExpr(value, variable, tag.value)
          ),
        };
      });
    }

    return {
      ...expandedQuery,
      adhocFilters: filters ?? [],
      query: this.templateSrv.replace(
        query.query ?? '',
        scopedVars,
        (value: string | string[] = [], variable: QueryVariableModel) =>
          this.interpolateQueryExpr(value, variable, query.query)
      ), // The raw sql query text
      rawSql: this.templateSrv.replace(
        query.rawSql ?? '',
        scopedVars,
        (value: string | string[] = [], variable: QueryVariableModel) =>
          this.interpolateQueryExpr(value, variable, query.rawSql)
      ), // The raw sql query text
      alias: this.templateSrv.replace(query.alias ?? '', scopedVars),
      limit: this.templateSrv.replace(query.limit?.toString() ?? '', scopedVars),
      measurement: this.templateSrv.replace(
        query.measurement ?? '',
        scopedVars,
        (value: string | string[] = [], variable: QueryVariableModel) =>
          this.interpolateQueryExpr(value, variable, query.measurement)
      ),
      policy: this.templateSrv.replace(query.policy ?? '', scopedVars),
      slimit: this.templateSrv.replace(query.slimit?.toString() ?? '', scopedVars),
      tz: this.templateSrv.replace(query.tz ?? '', scopedVars),
    };
  }

  interpolateQueryExpr(value: string | string[] = [], variable: QueryVariableModel, query?: string) {
    // Numbers need no escaping in any dialect
    if (typeof value === 'string' && !isNaN(parseFloat(value))) {
      return value;
    }

    // SQL has no regex literals, so regex escaping would corrupt values.
    // Multiple values are quoted and comma-joined for use with IN ($var).
    if (this.version === InfluxVersion.SQL) {
      if (typeof value === 'string') {
        return value;
      }
      if (value.length === 1) {
        return value[0];
      }
      return value.map((v) => `'${v.replace(/'/g, "''")}'`).join(', ');
    }

    // Multi-value variables always interpolate to an (a|b) alternation,
    // which is only usable inside a regex, so always escape them
    if (variable.multi) {
      if (typeof value === 'string') {
        return escapeRegex(value);
      }
      return `(${value.map((v) => escapeRegex(v)).join('|')})`;
    }

    // Legacy callers can pass a non-string query (e.g. templateSrv's default
    // format function), in which case the usage context is unknown
    if (!query || typeof query !== 'string' || !variable?.name) {
      return value;
    }

    // Single-value variables are only escaped when used inside a regex
    if (isVariableInRegexLiteral(variable.name, query)) {
      return typeof value === 'string' ? escapeRegex(value) : `(${value.map((v) => escapeRegex(v)).join('|')})`;
    }

    return value;
  }

  async runMetadataQuery(target: InfluxQuery): Promise<MetricFindValue[]> {
    return lastValueFrom(
      super.query({
        targets: [target],
      } as DataQueryRequest)
    ).then(this.toMetricFindValue);
  }

  async metricFindQuery(query: InfluxVariableQuery, options?: any): Promise<MetricFindValue[]> {
    if (this.version === InfluxVersion.Flux || this.version === InfluxVersion.SQL || this.access === 'proxy') {
      const target: InfluxQuery & SQLQuery = {
        refId: 'metricFindQuery',
        query: query.query,
        rawQuery: true,
        ...(this.version === InfluxVersion.SQL ? { rawSql: query.query, format: QueryFormat.Table } : {}),
      };
      return lastValueFrom(
        super.query({
          ...(options ?? {}), // includes 'range'
          maxDataPoints: query.maxDataPoints,
          targets: [target],
        })
      ).then(this.toMetricFindValue);
    }

    const interpolated = this.templateSrv.replace(
      query.query,
      options?.scopedVars,
      (value: string | string[] = [], variable: QueryVariableModel) =>
        this.interpolateQueryExpr(value, variable, query.query)
    );

    return lastValueFrom(this._seriesQuery(interpolated, options)).then((resp) => {
      return this.responseParser.parse(query.query, resp);
    });
  }

  toMetricFindValue(rsp: DataQueryResponse): MetricFindValue[] {
    const valueMap = new Map<string, MetricFindValue>();
    // Create MetricFindValue object for all frames
    rsp?.data?.forEach((frame: DataFrame) => {
      if (frame && frame.length > 0) {
        let field = frame.fields.find((f) => f.type === FieldType.string);
        if (!field) {
          field = frame.fields.find((f) => f.type !== FieldType.time);
        }
        if (field) {
          field.values.forEach((v) => {
            valueMap.set(v.toString(), { text: v.toString() });
          });
        }
      }
    });
    return Array.from(valueMap.values());
  }
  // By implementing getTagKeys and getTagValues we add ad-hoc filters functionality
  // Used in public/app/features/variables/adhoc/picker/AdHocFilterKey.tsx::fetchFilterKeys
  getTagKeys(options?: DataSourceGetTagKeysOptions<InfluxQuery>) {
    const query = buildMetadataQuery({
      type: 'TAG_KEYS',
      templateService: this.templateSrv,
      database: this.database,
      withTimeFilter: this.showTagTime,
    });

    return this.metricFindQuery({ refId: 'get-tag-keys', query });
  }

  getTagValues(options: DataSourceGetTagValuesOptions<InfluxQuery>) {
    const query = buildMetadataQuery({
      type: 'TAG_VALUES',
      templateService: this.templateSrv,
      database: this.database,
      withKey: options.key,
      withTimeFilter: this.showTagTime,
    });

    return this.metricFindQuery({ refId: 'get-tag-values', query });
  }

  /**
   * @deprecated
   */
  _seriesQuery(query: string, options?: any) {
    if (!query) {
      return of({ results: [] });
    }

    if (options && options.range) {
      const timeFilter = this.getTimeFilter({ rangeRaw: options.range, timezone: options.timezone });
      query = query.replace('$timeFilter', timeFilter);
    }

    return this._influxRequest(this.httpMode, '/query', { q: query, epoch: 'ms' }, options);
  }

  /**
   * @deprecated
   */
  serializeParams(params: any) {
    if (!params) {
      return '';
    }

    return reduce(
      params,
      (memo: string[], value, key) => {
        if (value === null || value === undefined) {
          return memo;
        }
        memo.push(encodeURIComponent(key) + '=' + encodeURIComponent(value));
        return memo;
      },
      []
    ).join('&');
  }

  /**
   * @deprecated
   */
  _influxRequest(method: string, url: string, data: any, options?: any) {
    const currentUrl = this.urls.shift()!;
    this.urls.push(currentUrl);

    const params: any = {};

    if (this.username) {
      params.u = this.username;
      params.p = this.password;
    }

    if (options && options.database) {
      params.db = options.database;
    } else if (this.database) {
      params.db = this.database;
    }

    if (options?.policy && options.policy !== DEFAULT_POLICY) {
      params.rp = options.policy;
    }

    const { q } = data;

    if (method === 'POST' && has(data, 'q')) {
      // verb is POST and 'q' param is defined
      extend(params, omit(data, ['q']));
      data = this.serializeParams(pick(data, ['q']));
    } else if (method === 'GET' || method === 'POST') {
      // verb is GET, or POST without 'q' param
      extend(params, data);
      data = null;
    }

    const req: any = {
      method: method,
      url: currentUrl + url,
      params: params,
      data: data,
      precision: 'ms',
      inspect: { type: 'influxdb' },
      paramSerializer: this.serializeParams,
    };

    req.headers = req.headers || {};
    if (this.basicAuth || this.withCredentials) {
      req.withCredentials = true;
    }
    if (this.basicAuth) {
      req.headers.Authorization = this.basicAuth;
    }

    if (method === 'POST') {
      req.headers['Content-type'] = 'application/x-www-form-urlencoded';
    }

    return getBackendSrv()
      .fetch(req)
      .pipe(
        map((result: FetchResponse) => {
          const { data } = result;
          if (data) {
            data.executedQueryString = q;
            if (data.results) {
              const errors = result.data.results.filter((elem: any) => elem.error);

              if (errors.length > 0) {
                throw {
                  message: 'InfluxDB Error: ' + errors[0].error,
                  data,
                };
              }
            }
          }
          return data;
        }),
        catchError((err) => {
          if (err.cancelled) {
            return of(err);
          }

          return throwError(this.handleErrors(err));
        })
      );
  }

  /**
   * @deprecated
   */
  handleErrors(err: any) {
    const error: DataQueryError = {
      message:
        (err && err.status) ||
        (err && err.message) ||
        'Unknown error during query transaction. Please check JS console logs.',
    };

    if ((Number.isInteger(err.status) && err.status !== 0) || err.status >= 300) {
      if (err.data && err.data.error) {
        error.message = 'InfluxDB Error: ' + err.data.error;
        error.data = err.data;
        // @ts-ignore
        error.config = err.config;
      } else {
        error.message = 'Network Error: ' + err.statusText + '(' + err.status + ')';
        error.data = err.data;
        // @ts-ignore
        error.config = err.config;
      }
    }

    return error;
  }

  getTimeFilter(options: { rangeRaw: RawTimeRange; timezone: string }) {
    const from = this.getInfluxTime(options.rangeRaw.from, false, options.timezone);
    const until = this.getInfluxTime(options.rangeRaw.to, true, options.timezone);

    return 'time >= ' + from + ' and time <= ' + until;
  }

  getInfluxTime(date: DateTime | string, roundUp: boolean, timezone: string) {
    let outPutDate;
    if (isString(date)) {
      if (date === 'now') {
        return 'now()';
      }

      const parts = /^now-(\d+)([dhms])$/.exec(date);
      if (parts) {
        const amount = parseInt(parts[1], 10);
        const unit = parts[2];
        return 'now() - ' + amount + unit;
      }
      outPutDate = dateMath.parse(date, roundUp, timezone);
      if (!outPutDate) {
        throw new Error('unable to parse date');
      }
      date = outPutDate;
    }

    return date.valueOf() + 'ms';
  }

  async annotationEvents(options: DataQueryRequest, annotation: InfluxQuery): Promise<AnnotationEvent[]> {
    if (this.version === InfluxVersion.Flux) {
      return Promise.reject({
        message: 'Flux requires the standard annotation query',
      });
    }

    // InfluxQL puts a query string on the annotation
    if (!annotation.query) {
      return Promise.reject({
        message: 'Query missing in annotation definition',
      });
    }

    if (this.access === 'proxy') {
      // We want to send our query to the backend as a raw query
      const target: InfluxQuery = {
        refId: 'metricFindQuery',
        datasource: this.getRef(),
        query: this.templateSrv.replace(
          annotation.query,
          undefined,
          (value: string | string[] = [], variable: QueryVariableModel) =>
            this.interpolateQueryExpr(value, variable, annotation.query)
        ),
        rawQuery: true,
      };

      return lastValueFrom(
        getBackendSrv()
          .fetch<BackendDataSourceResponse>({
            url: '/api/ds/query',
            method: 'POST',
            headers: this.getRequestHeaders(),
            data: {
              from: options.range.from.valueOf().toString(),
              to: options.range.to.valueOf().toString(),
              queries: [target],
            },
            requestId: annotation.name,
          })
          .pipe(
            map(
              async (res: FetchResponse<BackendDataSourceResponse>) =>
                await this.responseParser.transformAnnotationResponse(annotation, res, target)
            )
          )
      );
    }

    const timeFilter = this.getTimeFilter({ rangeRaw: options.range.raw, timezone: options.timezone });
    let query = annotation.query.replace('$timeFilter', timeFilter);
    query = this.templateSrv.replace(query, undefined, (value: string | string[] = [], variable: QueryVariableModel) =>
      this.interpolateQueryExpr(value, variable, query)
    );

    return lastValueFrom(this._seriesQuery(query, options)).then((data) => {
      if (!data || !data.results || !data.results[0]) {
        throw { message: 'No results in response from InfluxDB' };
      }
      return new InfluxSeries({
        series: data.results[0].series,
        annotation: annotation,
      }).getAnnotations();
    });
  }
}
