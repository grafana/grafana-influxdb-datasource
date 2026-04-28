// We set this specifically for 2 reasons.
// 1. It makes sense for both CI tests and local tests to behave the same so issues are found earlier
// 2. Any wrong timezone handling could be hidden if we use UTC/GMT local time (which would happen in CI).
process.env.TZ = 'Pacific/Easter'; // UTC-06:00 or UTC-05:00 depending on daylight savings

module.exports = {
  ...require('./.config/jest.config'),
  moduleNameMapper: {
    ...require('./.config/jest.config').moduleNameMapper,
    // Map monaco-editor exactly (not @monaco-editor/*) to prevent loading the
    // real ESM-only package in jsdom. Create src/__mocks__/monaco-editor.ts.
    '^monaco-editor$': '<rootDir>/src/__mocks__/monaco-editor.ts',
  },
  transformIgnorePatterns: [
    require('./.config/jest/utils').nodeModulesToTransform([
      ...require('./.config/jest/utils').grafanaESModules,
      'monaco-editor',
      // OpenFeature packages ship ESM-only; needed by @grafana/runtime
      '@openfeature/ofrep-web-provider',
      '@openfeature/web-sdk',
      // Lezer packages ship ESM-only; needed by @grafana/lezer-logql
      '@lezer/lr',
      '@lezer/common',
      '@lezer/highlight',
      '@grafana/lezer-logql',
    ]),
  ],
};
