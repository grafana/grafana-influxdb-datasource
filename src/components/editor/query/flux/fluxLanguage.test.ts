import * as monaco from 'monaco-editor/esm/vs/editor/editor.api';
import 'monaco-editor/esm/vs/editor/contrib/bracketMatching/browser/bracketMatching';
import 'monaco-editor/esm/vs/editor/contrib/comment/browser/comment';

import { FLUX_LANGUAGE_ID, registerFluxLanguage } from './fluxLanguage';

const createEditor = (value: string) =>
  monaco.editor.create(document.createElement('div'), { value, language: FLUX_LANGUAGE_ID });

const tokenTypeAt = (source: string, needle: string) => {
  const lines = source.split('\n');
  const lineIndex = lines.findIndex((line) => line.includes(needle));
  const column = lines[lineIndex].indexOf(needle);
  const tokens = monaco.editor.tokenize(source, FLUX_LANGUAGE_ID)[lineIndex];
  return tokens.filter(({ offset }) => offset <= column).pop()?.type;
};

describe('registerFluxLanguage', () => {
  beforeAll(() => registerFluxLanguage(monaco));

  it('toggles a line comment with the Flux // marker', () => {
    const editor = createEditor('from(bucket: "b")');

    editor.trigger('test', 'editor.action.commentLine', null);

    expect(editor.getValue()).toBe('// from(bucket: "b")');
    editor.dispose();
  });

  it('jumps to the matching bracket', () => {
    const editor = createEditor('filter(fn: (r) => r._value > 0)');
    editor.setPosition({ lineNumber: 1, column: 7 });

    editor.trigger('test', 'editor.action.jumpToBracket', null);

    expect(editor.getPosition()).toEqual(new monaco.Position(1, 31));
    editor.dispose();
  });

  it('registers the language once across editor mounts', () => {
    registerFluxLanguage(monaco);

    expect(monaco.languages.getLanguages().filter(({ id }) => id === FLUX_LANGUAGE_ID)).toHaveLength(1);
  });

  it.each([
    ['a line comment', '// v.windowPeriod is a variable', '//', 'comment.line.double-slash.flux'],
    ['a trailing comment', 'from(bucket: "b") // note', '// note', 'comment.line.double-slash.flux'],
    ['// inside a string', 'url = "http://example.com"', '//example', 'string.quoted.double.flux'],
    ['a multi-line string', 's = "first\n// still the string"', '// still', 'string.quoted.double.flux'],
    ['an escape in a string', 's = "a\\"b"', '\\"', 'constant.character.escape.flux'],
    ['a quote inside a regex', 'r._value =~ /a"b/', '"b', 'string.regexp.flux'],
    ['a regex that starts with ^', 'r._field =~ /^cpu/', '/^', 'string.regexp.flux'],
    [
      'the line after a regex that ends with +',
      'r.cpu =~ /cpu[0-9]+/)\n  |> yield()',
      'yield',
      'support.function.flux',
    ],
    ['unspaced division', 'r.a/r.b + c/d', '/r.b', ''],
    ['a control keyword', 'import "strings"', 'import', 'keyword.control.flux'],
    ['a keyword inside an identifier', 'within = 1', 'in =', ''],
    ['the pipe-forward operator', '  |> yield()', '|>', 'keyword.operator.new.flux'],
    ['a logical operator', 'r.a == 1 and r.b == 2', 'and', 'keyword.operator.flux'],
    ['a boolean', 'predicate: (r) => true', 'true', 'constant.language.flux'],
    ['a duration', 'range(start: -1h)', '1h', 'constant.other.flux'],
    ['an identifier named like a duration unit', '(m) => m.x', 'm.x', ''],
    ['a date', 'range(start: 2026-01-01T00:00:00Z)', '2026', 'constant.other.flux'],
    ['a number', 'limit(n: 42)', '42', 'constant.numeric.flux'],
    ['a function call', '  |> aggregateWindow(every: v.windowPeriod)', 'aggregateWindow', 'support.function.flux'],
    ['a fn parameter', 'filter(fn: (r) => true)', 'fn:', 'support.function.flux'],
  ])('tokenizes %s', (_, source, needle, expected) => {
    expect(tokenTypeAt(source, needle)).toBe(expected);
  });
});
