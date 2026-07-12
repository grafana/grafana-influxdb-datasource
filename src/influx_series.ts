import { each, includes, flatten } from 'lodash';

import { type QueryResultMeta } from '@grafana/data';

import { type InfluxQuery } from './types';

export default class InfluxSeries {
  refId?: string;
  series: any;
  alias?: string;
  annotation?: InfluxQuery;
  meta?: QueryResultMeta;

  constructor(options: {
    series: any;
    alias?: string;
    annotation?: InfluxQuery;
    meta?: QueryResultMeta;
    refId?: string;
  }) {
    this.series = options.series;
    this.alias = options.alias;
    this.annotation = options.annotation;
    this.meta = options.meta;
    this.refId = options.refId;
  }

  getAnnotations() {
    const list: any[] = [];

    each(this.series, (series) => {
      let titleCol: any = null;
      let timeCol: any = null;
      let timeEndCol: any = null;
      const tagsCol: string[] = [];
      let textCol: any = null;

      each(series.columns, (column, index) => {
        if (column === 'time') {
          timeCol = index;
          return;
        }
        if (column === 'sequence_number') {
          return;
        }
        if (column === this.annotation?.titleColumn) {
          titleCol = index;
          return;
        }
        if (includes((this.annotation?.tagsColumn || '').replace(' ', '').split(','), column)) {
          tagsCol.push(index);
          return;
        }
        if (column === this.annotation?.textColumn) {
          textCol = index;
          return;
        }
        if (column === this.annotation?.timeEndColumn) {
          timeEndCol = index;
          return;
        }
        // legacy case
        if (!titleCol && textCol !== index) {
          titleCol = index;
        }
      });

      each(series.values, (value) => {
        const data = {
          annotation: this.annotation,
          time: +new Date(value[timeCol]),
          title: value[titleCol],
          timeEnd: value[timeEndCol],
          // Remove empty values, then split in different tags for comma separated values
          tags: flatten(
            tagsCol
              .filter((t) => {
                return value[t];
              })
              .map((t) => {
                return value[t].split(',');
              })
          ),
          text: value[textCol],
        };

        list.push(data);
      });
    });

    return list;
  }
}
