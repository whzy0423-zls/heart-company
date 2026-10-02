import { TYPES_INFO, CENTERS } from '../../data/enneagramGame.js'
import { PAIRS_1_3 } from './data/pairs-1-3.js'
import { PAIRS_4_9 } from './data/pairs-4-9.js'
import { buildRelationSources, COMMUNICATION_TOOLS } from './data/relationSources.js'

const PAIR_DETAILS = { ...PAIRS_1_3, ...PAIRS_4_9 }

// 相处提示供双方核对，不以型号判断关系好坏或计算匹配分数。
const PROFILES = {
  1: {
    need: '努力被看见，重要的约定有清楚的标准。',
    support: '先肯定已经做好的部分，再一起商量什么程度就够了。',
    stress: '可能紧盯细节，把建议说成了批评。',
    gift: '认真负责', strength: '把模糊的约定整理成可执行的步骤，照顾容易漏掉的细节。',
    ask: '我想先把我们在意的标准说清楚，也愿意听听哪些可以放松',
    reply: '我看见你做的准备了，我们一起确定这次做到哪一步就好。',
    action: '一起选出一件不必做到完美的事，约定完成的标准',
  },
  2: {
    need: '付出被珍惜，也能安心说出自己的需要。',
    support: '感谢一件具体的事，主动问需要什么，不默认对方总能照顾大家。',
    stress: '可能一直照顾别人，却把自己的委屈藏起来。',
    gift: '体贴连接', strength: '留意细小的情绪与需要，让日常相处多一份主动的关心。',
    ask: '我也有需要被照顾的时候，我会试着直接告诉你',
    reply: '谢谢你为我做的这些。今天你希望我帮你做什么？说不也没关系。',
    action: '问一件对方真正想被照顾的事，做之前先确认',
  },
  3: {
    need: '能力被肯定，暂时停下来时也被重视。',
    support: '欣赏具体的投入，也留出一段不用谈成绩的相处时间。',
    stress: '可能急着推进结果，略过了自己和对方的感受。',
    gift: '推动进展', strength: '把愿望拆成目标和行动，让共同计划逐步落地。',
    ask: '我习惯先想怎么推进，也想练习说说过程里的感受',
    reply: '我欣赏你的投入，即使今天没有成果，我也愿意陪你待一会儿。',
    action: '留十分钟只聊近况，不评价效率与成绩',
  },
  4: {
    need: '真实的感受被理解，独特的表达被认真对待。',
    support: '先听完整，再复述听到的感受；给建议前先问一句。',
    stress: '可能反复琢磨失落，觉得自己的感受没有被理解。',
    gift: '感受细腻', strength: '发现关系里不易说出口的感受，为日常带来真诚的表达。',
    ask: '我想先把感受说完，等我准备好再一起找办法',
    reply: '我想听懂这件事对你意味着什么。你想让我先听，还是一起想办法？',
    action: '听完一段感受，再用自己的话复述并请对方确认',
  },
  5: {
    need: '有自主思考的空间，时间与精力的边界被尊重。',
    support: '说明要讨论的事，约好时间，允许想一想再回应。',
    stress: '可能退回独处，用沉默保护精力却没有说明原因。',
    gift: '深入观察', strength: '梳理复杂的信息，帮助双方区分事实、猜测与待确认的问题。',
    ask: '我需要一点时间想清楚，会和你约好回来继续聊的时间',
    reply: '你可以先想一想。我们约个都方便的时间再聊，好吗？',
    action: '提前说明一个要聊的问题，并约好讨论的时间',
  },
  6: {
    need: '约定可靠，遇到不确定时有人一起确认。',
    support: '把承诺说具体，变化时及时告知，一起准备可行的备用方案。',
    stress: '可能反复确认风险，把尚未发生的事当成眼前压力。',
    gift: '可靠守护', strength: '提前留意风险，记住共同的承诺，为计划多准备一条退路。',
    ask: '不确定时我会多问几句，我想和你一起确认事实',
    reply: '我们先分清已经知道和还没确定的事，有变化我会及时告诉你。',
    action: '把一个约定写清时间与做法，同时商量变化时怎么通知',
  },
  7: {
    need: '有选择与探索的空间，困难时也能保留希望。',
    support: '给出可选的做法，先接住难受，再一起寻找新的可能。',
    stress: '可能很快转移话题，用新计划跳过尚未处理的难受。',
    gift: '发现可能', strength: '为卡住的事情寻找新角度，也让共同生活多一些轻松体验。',
    ask: '我容易先找开心的办法，也愿意多停一会儿听难处',
    reply: '我们可以一起选个喜欢的做法；现在难受的部分，也可以慢慢说。',
    action: '从两个可选的小计划中选一个，先完成再添加新安排',
  },
  8: {
    need: '被坦诚对待，自己的决定与边界被尊重。',
    support: '直接说事实和立场，商量边界，不替对方做主。',
    stress: '可能提高音量、加快决定，让对方来不及表达。',
    gift: '果断担当', strength: '遇到难题愿意站出来，在尊重双方意愿时把力量用来支持彼此。',
    ask: '我说话有时会比较直接，也想听到你真实的立场',
    reply: '我会坦诚告诉你我的想法，也尊重你的选择，我们一起商量边界。',
    action: '各说一条愿意承担的事和一条需要被尊重的边界',
  },
  9: {
    need: '相处平和，自己的偏好也有被听见的位置。',
    support: '给足表达时间，邀请说出真实选择，不把沉默当作同意。',
    stress: '可能先答应或拖延，把不同意见压下去。',
    gift: '包容协调', strength: '容纳不同的看法，放缓紧绷的气氛，让双方有机会重新对话。',
    ask: '我有时需要慢一点说出想法，希望你也听听我的选择',
    reply: '这次我想听你的偏好。和我不一样也没关系，你可以慢慢说。',
    action: '让对方先选一件日常小事，认真听理由，不急着代替决定',
  },
}

const CENTER_PRACTICES = {
  gut: '遇到安排分歧，先各说一条底线，再各提一个愿意调整的地方；不要用谁先行动代替共同决定。',
  heart: '遇到情绪变化，先各说一个具体感受和一个请求；把“你不在乎我”换成“我希望今晚能聊十分钟”。',
  head: '遇到计划变化，把事实、担心和下一步分开说；先完成一个小行动，再决定是否需要更多准备。',
  'gut-heart': '例如分配家务时，先听感受，再谈分工；既说清“我希望被体谅”，也说清“这周各做哪一件”。',
  'gut-head': '例如临时改行程时，先确认必须守住的边界，再列一个备用方案，最后约好何时作决定。',
  'head-heart': '例如回复变慢时，先问真实情况，再说自己的感受；不要让信息不足变成对心意的猜测。',
}

function normalizeId(value) {
  if (typeof value === 'string' && /^[1-9]$/.test(value)) return Number(value)
  return Number.isInteger(value) && value >= 1 && value <= 9 ? value : null
}

export function buildRelationAnalysis(myId, taId) {
  const mine = normalizeId(myId)
  const other = normalizeId(taId)
  if (!mine || !other) return null
  const a = TYPES_INFO[mine]
  const b = TYPES_INFO[other]
  const me = PROFILES[mine]
  const ta = PROFILES[other]
  const same = mine === other
  const sameCenter = a.center === b.center
  const centerKey = sameCenter ? a.center : [a.center, b.center].sort().join('-')
  const pair = PAIR_DETAILS[[mine, other].sort((x, y) => x - y).join('-')]
  const needs = [[mine, '我', a, me], [other, 'TA', b, ta]].map(([typeId, role, info, profile]) => ({
    role, typeId, name: info.name, need: profile.need, support: profile.support, stress: profile.stress,
  }))
  return {
    pairInsight: { ...pair, scene: { ...pair.scene } },
    sources: buildRelationSources(mine, other),
    communicationTools: COMMUNICATION_TOOLS.map(tool => ({ ...tool })),
    label: same ? '同型照见' : sameCenter ? '同频共鸣' : '互补同行',
    bond: same
      ? `双方都可能重视${me.gift}，有机会理解彼此的用心。但同为${a.name}，经历与表达方式仍会不同，值得重新问一遍对方的需要。`
      : `${mine}号一方带来${me.gift}，${other}号一方带来${ta.gift}。${sameCenter ? `同属${CENTERS[a.center].name}，可以从熟悉的关注点开始交流。` : `从${CENTERS[a.center].name}到${CENTERS[b.center].name}，可以一起补充看事情的角度。`}`,
    friction: same
      ? `当两人都承受压力时，都${me.stress}先辨认这个信号，别把熟悉当成“我已经知道你在想什么”。`
      : `压力来时，${mine}号一方${me.stress}${other}号一方${ta.stress}这些反应不等于不在乎，先核对彼此正在经历什么。`,
    tip: same
      ? '轮流做先开口的人，每次只谈一件事。相似的偏好可以互相理解，不必要求对方和自己完全一样。'
      : `支持${other}号一方可以从这里开始：${ta.support}${mine}号一方也可以说清自己的需要：${me.need}`,
    myDrive: `${mine}号 ${a.name}：${a.desire}`,
    taDrive: `${other}号 ${b.name}：${b.desire}`,
    needs,
    strengths: same ? [
      { title: `共同的${me.gift}`, text: me.strength },
      { title: '把默契变成合作', text: '先说清各自愿意承担的部分，再约定需要帮助时怎么开口；相似不代表要承担同样的角色。' },
    ] : [
      { title: `${mine}号一方的${me.gift}`, text: me.strength },
      { title: `${other}号一方的${ta.gift}`, text: ta.strength },
    ],
    dialogue: [
      { role: '我对 TA 说', text: `${me.ask}。${ta.reply}` },
      { role: 'TA 对我说', text: `${ta.ask}。${me.reply}` },
    ],
    practices: [
      { title: '先把关注点说清楚', text: CENTER_PRACTICES[centerKey] },
      { title: '今天做一件支持彼此的小事', text: same ? `双方可以轮流试着${me.action}；先问对方这是否正是需要的。` : `${mine}号一方可以${ta.action}；${other}号一方可以${me.action}。先确认意愿，再付诸行动。` },
      { title: '约一次十分钟的复盘', text: '各说一件被理解的时刻、一处还想调整的地方，再共同选一个下次尝试的小动作。只谈具体事情，不用型号给彼此下结论。' },
    ],
  }
}
