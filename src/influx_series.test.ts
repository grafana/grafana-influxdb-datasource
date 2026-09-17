import InfluxSeries from './influx_series';

describe('when generating annotations from influxdb response', () => {
  describe('with empty tagsColumn', () => {
    const options = {
      alias: '',
      annotation: {
        refId: '',
      },
      series: [
        {
          name: 'logins.count',
          tags: { datacenter: 'Africa', server: 'server2' },
          columns: ['time', 'datacenter', 'hostname', 'source', 'value'],
          values: [[1481549440372, 'America', '10.1.100.10', 'backend', 215.7432653659507]],
        },
      ],
    };

    it('should multiple tags', () => {
      const series = new InfluxSeries(options);
      const annotations = series.getAnnotations();

      expect(annotations[0].tags.length).toBe(0);
    });
  });

  describe('given annotation response', () => {
    const options = {
      alias: '',
      annotation: {
        tagsColumn: 'datacenter, source',
        refId: '',
      },
      series: [
        {
          name: 'logins.count',
          tags: { datacenter: 'Africa', server: 'server2' },
          columns: ['time', 'datacenter', 'hostname', 'source', 'value'],
          values: [[1481549440372, 'America', '10.1.100.10', 'backend', 215.7432653659507]],
        },
      ],
    };

    it('should multiple tags', () => {
      const series = new InfluxSeries(options);
      const annotations = series.getAnnotations();

      expect(annotations[0].tags.length).toBe(2);
      expect(annotations[0].tags[0]).toBe('America');
      expect(annotations[0].tags[1]).toBe('backend');
    });
  });
});
