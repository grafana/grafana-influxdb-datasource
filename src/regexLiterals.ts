import { escapeRegex } from '@grafana/data';

// InfluxQL keywords that can stand directly before a regex literal (SELECT,
// FROM, GROUP BY). Any other bare word is an operand, so a `/` after it is
// division. Flux regex literals follow an operator or punctuation in practice,
// and Flux keywords such as `then` are legal bare identifiers in InfluxQL.
const KEYWORDS_BEFORE_REGEX = new Set(['select', 'from', 'by']);

const WORD_CHAR = /[\p{L}\p{N}_$]/u;

interface DelimitedSpan {
  body: string;
  next: number;
}

// Scans a `\`-escaped span that opens at `start` and closes at `delimiter`.
// Regex literals cannot contain a newline in either language, so `singleLine`
// ends an unterminated one at the newline. Other unterminated spans run to the
// end of the input.
const scanDelimited = (query: string, start: number, delimiter: string, singleLine: boolean): DelimitedSpan => {
  let i = start + 1;
  while (i < query.length && query[i] !== delimiter && !(singleLine && query[i] === '\n')) {
    i += query[i] === '\\' ? 2 : 1;
  }
  return { body: query.slice(start + 1, i), next: query[i] === delimiter ? i + 1 : i };
};

// Collects the body of every regex literal in an InfluxQL or Flux query.
// InfluxQL accepts regex literals as SELECT fields, FROM sources (also after
// `db.rp.`), GROUP BY dimensions, function arguments and after `=~` / `!~`.
// Flux accepts them wherever an expression can start. In both languages a `/`
// after an operand is division, so a regex literal opens only when the
// previous token is not an operand. Strings, quoted identifiers and comments
// are skipped so a `/` inside them never opens a regex.
const findRegexLiterals = (query: string): string[] => {
  const literals: string[] = [];
  let afterOperand = false;
  let i = 0;

  while (i < query.length) {
    const ch = query[i];

    if (/\s/.test(ch)) {
      i++;
    } else if (query.startsWith('--', i) || query.startsWith('//', i)) {
      const newline = query.indexOf('\n', i);
      i = newline === -1 ? query.length : newline;
    } else if (query.startsWith('/*', i)) {
      const close = query.indexOf('*/', i + 2);
      i = close === -1 ? query.length : close + 2;
    } else if (ch === "'" || ch === '"') {
      i = scanDelimited(query, i, ch, false).next;
      afterOperand = true;
    } else if (ch === '/' && !afterOperand) {
      const literal = scanDelimited(query, i, '/', true);
      literals.push(literal.body);
      i = literal.next;
      afterOperand = true;
    } else if (WORD_CHAR.test(ch)) {
      const start = i;
      while (i < query.length && WORD_CHAR.test(query[i])) {
        i++;
      }
      const word = query.slice(start, i);
      // A number keeps its trailing `.` (Flux allows `0.`), and a word after
      // `.` is a member name or path segment. Neither is a keyword.
      if (/^\d+$/.test(word) && query[i] === '.') {
        i++;
      }
      afterOperand = query[start - 1] === '.' || !KEYWORDS_BEFORE_REGEX.has(word.toLowerCase());
    } else {
      // Closing brackets end an operand. Every other symbol is an operator or punctuation.
      afterOperand = ch === ')' || ch === ']' || ch === '}';
      i++;
    }
  }

  return literals;
};

/**
 * Decides whether a variable is referenced inside a regex literal in the
 * given query text, so callers know to regex-escape its value.
 */
export const isVariableInRegexLiteral = (name: string, query: string): boolean => {
  const escapedName = escapeRegex(name);
  // Matches $name and ${name} / ${name:format} references
  const varRef = new RegExp(`\\$(?:${escapedName}\\b|\\{${escapedName}(?::[^}]*)?\\})`);
  return findRegexLiterals(query).some((literal) => varRef.test(literal));
};
