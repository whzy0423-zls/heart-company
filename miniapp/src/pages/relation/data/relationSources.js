// Reviewed 2026-10-02. Pair summaries are original Chinese paraphrases;
// practice scenes are editorial examples, not cases reported by these authors.
const CHECKED_AT = '2026-10-02'
const COMMON_SOURCES = [
  {
    id: 'gentle-startup', kind: '沟通方法', publisher: 'The Gottman Institute',
    title: 'How to Fight Smarter: Soften Your Start-Up',
    url: 'https://www.gottman.com/blog/softening-startup/',
    note: '从具体事件、感受和请求开场，避免把不满变成人格评价。练习话术为中文原创整理。',
  },
  {
    id: 'conflict-repair', kind: '沟通方法', publisher: 'The Gottman Institute',
    title: 'The Four Horsemen: The Antidotes',
    url: 'https://www.gottman.com/blog/the-four-horsemen-the-antidotes/',
    note: '参考其中对欣赏、承担责任和暂停平复的建议；文章主要讨论伴侣沟通。',
  },
  {
    id: 'evidence-review', kind: '研究综述', publisher: 'Hook 等 · Journal of Clinical Psychology · 2021',
    title: 'The Enneagram: A systematic review of the literature and directions for future research',
    url: 'https://doi.org/10.1002/jclp.23097',
    note: '已核对论文摘要与书目信息。综述指出九型的信度、效度证据不一致；本页用于交流与反思，不据此预测关系成败。',
  },
]

export function buildRelationSources(first, second) {
  const [a, b] = [first, second].sort((x, y) => x - y)
  return [{
    id: 'pair-theory', kind: '九型理论', publisher: 'The Enneagram Institute',
    title: `Relationship Type ${a} with Type ${b}`,
    url: `https://www.enneagraminstitute.com/relationship-type-${a}-with-type-${b}/`,
    note: '本组互补特点与摩擦循环的参考原文。中文内容为简要转述，生活情境为原创练习，并非真实个案。',
  }, ...COMMON_SOURCES].map(source => ({ ...source, checkedAt: CHECKED_AT }))
}

export const COMMUNICATION_TOOLS = [
  {
    id: 'start', sourceId: 'gentle-startup', title: '把指责换成具体请求',
    text: '先描述一件发生的事，再说感受和希望。少用“总是、从不”，一次只谈一个问题。',
    example: '“刚才计划临时变了，我有些着急。下次能提前一起确认吗？”',
  },
  {
    id: 'appreciate', sourceId: 'conflict-repair', title: '先看见对方做过的事',
    text: '减少讥讽与比较，具体表达感谢。欣赏可以和不同意见同时存在。',
    example: '“谢谢你记得这件事。做法上我有另一个想法，愿意一起听听吗？”',
  },
  {
    id: 'own-part', sourceId: 'conflict-repair', title: '先承担自己能改的一部分',
    text: '解释前先承认具体疏漏，再提出补救。承担自己的部分，不等于包揽全部责任。',
    example: '“这次我没有及时说明，让你多等了。以后变动时我会先告知。”',
  },
  {
    id: 'pause', sourceId: 'conflict-repair', title: '约好暂停，也约好回来',
    text: '情绪太满时先暂停，留出平复时间并约定何时继续，避免无说明地消失。',
    example: '“我现在有些激动，先休息一下。今晚八点再聊，可以吗？”',
  },
]
