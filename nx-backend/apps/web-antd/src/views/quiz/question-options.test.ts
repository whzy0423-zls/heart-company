import { describe, expect, it } from 'vitest';

import { parseAndValidateQuizOptions } from './question-options';

describe('parseAndValidateQuizOptions', () => {
  it('rejects enabled question options without numeric enneagram weights', () => {
    expect(() =>
      parseAndValidateQuizOptions(
        JSON.stringify([
          { id: 'a', text: '选项 A', weights: {} },
          { id: 'b', text: '选项 B' },
        ]),
        'enabled',
      ),
    ).toThrow('启用题目的每个选项都必须配置有效 weights');
  });

  it('rejects fewer than two complete options', () => {
    expect(() =>
      parseAndValidateQuizOptions(
        JSON.stringify([{ id: 'a', text: '选项 A', weights: {} }]),
        'disabled',
      ),
    ).toThrow('请至少填写两个完整选项');
  });

  it('rejects duplicate option ids', () => {
    expect(() =>
      parseAndValidateQuizOptions(
        JSON.stringify([
          { id: 'a', text: '选项 A', weights: {} },
          { id: 'a', text: '选项 B', weights: {} },
        ]),
        'disabled',
      ),
    ).toThrow('选项 id 不能重复');
  });

  it('accepts disabled question options with empty weights for draft cleanup', () => {
    expect(
      parseAndValidateQuizOptions(
        JSON.stringify([
          { id: 'a', text: '选项 A', weights: {} },
          { id: 'b', text: '选项 B', weights: {} },
        ]),
        'disabled',
      ),
    ).toEqual([
      { id: 'a', text: '选项 A', weights: {} },
      { id: 'b', text: '选项 B', weights: {} },
    ]);
  });

  it('normalizes valid numeric weights', () => {
    expect(
      parseAndValidateQuizOptions(
        JSON.stringify([
          { id: 'a', text: '选项 A', weights: { '1': 2, 9: '1' } },
          { id: 'b', text: '选项 B', weights: { '2': 1 } },
        ]),
        'enabled',
      ),
    ).toEqual([
      { id: 'a', text: '选项 A', weights: { 1: 2, 9: 1 } },
      { id: 'b', text: '选项 B', weights: { 2: 1 } },
    ]);
  });
});
