import { type AnnotationQuery, type AnnotationSupport } from '@grafana/data';

import { type InfluxQuery, type InfluxQueryTag } from './types';

type LegacyAnnotation = {
  query?: string;
  queryType?: string;
  fromAnnotations?: boolean;
  tagsColumn?: string;
  textColumn?: string;
  timeEndColumn?: string;
  titleColumn?: string;
  name?: string;
  target?: {
    limit?: string | number | undefined;
    tags?: InfluxQueryTag[];
  };
};

// this becomes the target in the migrated annotations
const migrateLegacyAnnotation = (json: LegacyAnnotation) => {
  // eslint-ignore-next-line
  const target: InfluxQuery = {
    refId: '',
    query: json.query ?? '',
    queryType: 'tags',
    fromAnnotations: true,
    tagsColumn: json.tagsColumn ?? '',
    textColumn: json.textColumn ?? '',
    timeEndColumn: json.timeEndColumn ?? '',
    titleColumn: json.titleColumn ?? '',
    name: json.name ?? '',
  };

  // handle json target fields
  if (json.target && json.target.limit) {
    target.limit = json.target.limit;
  }

  if (json.target && json.target.tags) {
    target.tags = json.target.tags;
  }

  return target;
};

export const prepareAnnotation: AnnotationSupport<InfluxQuery, AnnotationQuery<InfluxQuery>>['prepareAnnotation'] = (
  json
) => {
  // make sure that any additional target fields are migrated
  json.target = json.target && !json.target?.query ? migrateLegacyAnnotation(json) : json.target;

  return json;
};
