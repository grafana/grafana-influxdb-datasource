// Code based on https://github.com/influxdata/vsflux/tree/1198e3efbf9013988735398da42f950eefe1eb62
// MIT License Copyright (c) 2019 InfluxData
import { type Monaco, type monacoTypes } from '@grafana/ui';

export const FLUX_LANGUAGE_ID = 'influxdb-flux';

const conf: monacoTypes.languages.LanguageConfiguration = {
  comments: { lineComment: '//' },
  // Not in vsflux. Without it, Monaco does no bracket matching.
  brackets: [
    ['{', '}'],
    ['[', ']'],
    ['(', ')'],
  ],
  autoClosingPairs: [
    { open: '"', close: '"' },
    { open: '[', close: ']' },
    { open: "'", close: "'" },
    { open: '{', close: '}' },
    { open: '(', close: ')' },
  ],
};

const language: monacoTypes.languages.IMonarchLanguage = {
  defaultToken: '',
  tokenPostfix: '.flux',
  tokenizer: {
    root: [
      [/\/\/.*$/, 'comment.line.double-slash'],
      [/\b(in|import|package|return|option|builtin|test|testcase|if|then|else|exists)\b/, 'keyword.control'],
      [/\B(\+|-|\*|\/|%|\^|==|<|>|!=|=~|!~|=|=>|<-)\B/, 'keyword.operator'],
      [/\b(and|or)\b/, 'keyword.operator'],
      [/\|>/, 'keyword.operator.new'],
      [/\b(true|false|empty)\b/, 'constant.language'],
      [/\b(\d{4}-\d{2}-\d{2}(T\d{2}:\d{2}:\d{2}(\.\d*)?((Z|(\+|-)\d{2}:\d{2}))?)?)\b/, 'constant.other'],
      [/\b(\d*(y|mo|w|d|h|m|s|ms|us|µs|ns))+\b/, 'constant.other'],
      [/\b((0(x|X)[0-9a-fA-F]*)|((\d+\.?\d*)|(\.\d+)))\b/, 'constant.numeric'],
      [/\b(from|to|buckets|join)\b/, 'support.function'],
      [/\w+(?=\()/, 'support.function'],
      [/\b(fn)(?=:)\b/, 'support.function'],
      [/"/, { token: 'string.quoted.double', next: '@string' }],
      [/\/(?=\S.*\/)/, { token: 'string.regexp', next: '@regexp' }],
      // Not in vsflux. Monarch has no lookbehind, so this rule consumes whole words to stop \b rules matching mid-identifier.
      [/\w+/, ''],
    ],
    string: [
      [/\\./, 'constant.character.escape'],
      [/"/, { token: 'string.quoted.double', next: '@pop' }],
      [/[^\\"]+/, 'string.quoted.double'],
    ],
    regexp: [
      [/[^\\/+]\//, { token: 'string.regexp', next: '@pop' }],
      [/./, 'string.regexp'],
    ],
  },
};

/** Registers the Flux language on the shared Monaco instance, once per page. */
export const registerFluxLanguage = (monaco: Monaco) => {
  if (monaco.languages.getLanguages().some(({ id }) => id === FLUX_LANGUAGE_ID)) {
    return;
  }
  monaco.languages.register({ id: FLUX_LANGUAGE_ID });
  monaco.languages.setLanguageConfiguration(FLUX_LANGUAGE_ID, conf);
  monaco.languages.setMonarchTokensProvider(FLUX_LANGUAGE_ID, language);
};
