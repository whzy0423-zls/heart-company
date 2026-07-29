import { describe, expect, it } from 'vitest';

import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';

const viewsRoot = join(__dirname);

const formTags = [
  'Input',
  'Input.TextArea',
  'Input.Password',
  'InputNumber',
  'Textarea',
  'Select',
  'DatePicker',
  'DatePicker.RangePicker',
  'RangePicker',
  'AutoComplete',
  'Cascader',
  'TreeSelect',
  'a-input',
  'a-input-password',
  'a-textarea',
  'a-input-number',
  'a-select',
  'a-date-picker',
  'a-range-picker',
  'a-auto-complete',
  'a-cascader',
  'a-tree-select',
];

function walk(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    const stat = statSync(path);
    if (stat.isDirectory()) return walk(path);
    return path.endsWith('.vue') ? [path] : [];
  });
}

function readElement(source: string, start: number) {
  let quote: string | undefined;
  let end = start;
  while (end < source.length) {
    const char = source[end];
    if (quote) {
      if (char === quote && source[end - 1] !== '\\') quote = undefined;
    } else if (char === '"' || char === "'" || char === '`') {
      quote = char;
    } else if (char === '>') {
      return source.slice(start, end + 1);
    }
    end += 1;
  }
  return source.slice(start);
}

function lineNumber(source: string, index: number) {
  return source.slice(0, index).split('\n').length;
}

function hasAttr(block: string, attr: string) {
  const attrPattern = new RegExp(`(?:^|\\s)(?::|v-bind:)?${attr}(?:\\s*=|\\s|$)`, 'i');
  return attrPattern.test(block);
}

function isStaticDisabledOrReadonly(block: string) {
  return /(?:^|\s)(disabled|readonly)(?:\s|>|\/|$)/i.test(block);
}

function isNativeInputWithoutPlaceholderSupport(block: string) {
  return /<input\b/i.test(block) && /type=["'](?:file|date|hidden|checkbox|radio)["']/i.test(block);
}

function isSelectOption(block: string) {
  return /^<\/?(?:a-select-option|Select\.Option)\b/i.test(block.trim());
}

describe('view form placeholders', () => {
  it('keeps every editable form control with placeholder guidance', () => {
    expect(existsSync(viewsRoot)).toBe(true);

    const tagPattern = new RegExp(
      `<(${formTags
        .map((tag) => tag.replace(/[.]/g, '\\.').replace(/-/g, '\\-'))
        .join('|')})\\b`,
      'g',
    );

    const missing = walk(viewsRoot).flatMap((file) => {
      const source = readFileSync(file, 'utf8');
      const entries: string[] = [];
      for (const match of source.matchAll(tagPattern)) {
        const block = readElement(source, match.index ?? 0);
        if (
          isSelectOption(block) ||
          isStaticDisabledOrReadonly(block) ||
          isNativeInputWithoutPlaceholderSupport(block) ||
          hasAttr(block, 'placeholder') ||
          hasAttr(block, 'aria-label')
        ) {
          continue;
        }
        entries.push(`${relative(viewsRoot, file)}:${lineNumber(source, match.index ?? 0)} ${block.replace(/\s+/g, ' ').slice(0, 160)}`);
      }
      return entries;
    });

    expect(missing).toEqual([]);
  });
});
