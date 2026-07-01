package quiz

// DailyPracticeItem 每日成长练习的一条内容。
type DailyPracticeItem struct {
	Practice string `json:"practice"` // 今日练习：一个轻量的成长动作
	MindWord string `json:"mindWord"` // 今日心语：一句陪伴式的提醒
	Question string `json:"question"` // 今日适合问的问题：可向专属模型发起的提问
}

// DailyPractices 按主型组织的每日练习内容（id → 多条，按天轮换）。
// 与 TypeResults 同源，App 不内嵌规则，统一由后端下发。
var DailyPractices = map[int][]DailyPracticeItem{
	1: {
		{Practice: "今天为自己留出 10 分钟，做一件「足够好」而非「完美」的小事，完成后不再修改。", MindWord: "你不必做到无可挑剔，才值得被肯定。", Question: "我内心的批评声音是从哪里来的？"},
		{Practice: "记录一件今天你想纠正、但最终选择放过的事，体会放手的感受。", MindWord: "放过别人，也是放过那个紧绷的自己。", Question: "如何区分「原则」和「苛求」？"},
		{Practice: "给自己安排一段纯粹的休息，不带任何「应该」，只做让你放松的事。", MindWord: "休息不是偷懒，是让你走得更远的方式。", Question: "我允许自己犯哪些错误？"},
	},
	2: {
		{Practice: "今天对自己说一次「不」，把一次本想付出的精力留给自己。", MindWord: "先把自己照顾好，你的爱才会更长久。", Question: "我有哪些一直被我忽略的需求？"},
		{Practice: "向一个人坦诚说出你此刻真正想要的东西，而不是先去满足对方。", MindWord: "你的需求和别人的需求一样重要。", Question: "我是在给予，还是在期待回报？"},
		{Practice: "记录今天有谁照顾了你，允许自己安心地接受这份善意。", MindWord: "被人照顾，不是亏欠，是关系的流动。", Question: "我害怕不被需要时会发生什么？"},
	},
	3: {
		{Practice: "今天有一刻不去想「这样别人会怎么看」，只问自己「我想要什么」。", MindWord: "你的价值，不取决于你完成了多少。", Question: "我追逐的目标，是我想要的还是别人期待的？"},
		{Practice: "向亲近的人分享一个你不那么「成功」的真实片段。", MindWord: "真实的你，比完美的形象更动人。", Question: "卸下角色后，我是谁？"},
		{Practice: "今天留 15 分钟什么都不做，单纯地存在，而非产出。", MindWord: "你不需要一直证明自己，也值得被爱。", Question: "如果不再追求认可，我会做什么？"},
	},
	4: {
		{Practice: "把今天一种强烈的情绪，用文字或图画表达出来，而不沉溺其中。", MindWord: "你的深情是礼物，不是负担。", Question: "如何在感受情绪时不被它淹没？"},
		{Practice: "完成一件具体的小事，让丰富的内心落到行动上。", MindWord: "把感受变成行动，意义就生长出来了。", Question: "我羡慕别人的，其实是我内心渴望的什么？"},
		{Practice: "今天关注一个平凡的当下时刻，而不去比较或想象别处。", MindWord: "你已经完整，不缺少任何东西。", Question: "我一直在寻找的「真实自我」是什么样子？"},
	},
	5: {
		{Practice: "今天主动参与一次互动，分享一个你的想法，而不只是观察。", MindWord: "你的存在和观点，对他人很有价值。", Question: "我在用「思考」回避哪些「感受」？"},
		{Practice: "做一件需要投入身体和热情的事，哪怕只有 5 分钟。", MindWord: "走出头脑，生活会回应你的参与。", Question: "我囤积能量，是在害怕被消耗什么？"},
		{Practice: "向一个人多迈一步：主动联系，或者多停留一会儿。", MindWord: "适度靠近，不会耗尽你，反而滋养你。", Question: "我需要多少独处，才能真正感到安全？"},
	},
	6: {
		{Practice: "今天做一个小决定，只凭自己的判断，不去征求他人意见。", MindWord: "你比自己以为的更有能力。", Question: "我的焦虑，有多少真的会发生？"},
		{Practice: "写下一件你担心的事，再写下「最坏也不过如此」的应对方式。", MindWord: "确定感不在外面，而在你心里。", Question: "我把安全感交给了谁？"},
		{Practice: "今天信任一次直觉，哪怕没有十足把握也先行动。", MindWord: "怀疑会拖住你，行动会带你前进。", Question: "如果我足够安全，我会做什么不同的选择？"},
	},
	7: {
		{Practice: "今天专注完成一件事，过程中不切换、不开新计划。", MindWord: "深度，比新鲜更让人满足。", Question: "我用忙碌和计划，在逃避什么感受？"},
		{Practice: "允许自己停留在一种不那么舒服的情绪里 5 分钟，不急着转移。", MindWord: "痛苦也是体验的一部分，不必绕开。", Question: "如果不去追逐下一个刺激，我会感到什么？"},
		{Practice: "对一件已经开始的事做出一个小承诺，并今天兑现它。", MindWord: "扎根当下，自由才有重量。", Question: "我真正渴望的满足，是什么样子的？"},
	},
	8: {
		{Practice: "今天在一段关系里展示一点柔软：示弱、求助，或说出在意。", MindWord: "真正的强大，包含敢于温柔。", Question: "我用强势，在保护内心的什么？"},
		{Practice: "把一次掌控让渡出去，信任别人来处理一件事。", MindWord: "放下掌控，你会遇见更深的连接。", Question: "我害怕被控制时，身体有什么感觉？"},
		{Practice: "今天用你的力量去照顾一个人，而不是去赢一件事。", MindWord: "力量最好的去处，是守护而非压制。", Question: "在谁面前，我才敢卸下盔甲？"},
	},
	9: {
		{Practice: "今天明确说出一次自己的偏好，哪怕只是「我想吃这个」。", MindWord: "你的声音，值得被听见。", Question: "我回避冲突时，丢掉了自己的什么？"},
		{Practice: "为自己今天最重要的一件事排第一，并先去做它。", MindWord: "优先自己，不是自私，是清醒。", Question: "我真正想要的，是什么？"},
		{Practice: "今天觉察一次「随便都行」的瞬间，停下来问自己真实的想法。", MindWord: "你的存在本身，就很重要。", Question: "我用平静，在掩盖哪些没说出口的情绪？"},
	},
}

// DailyPracticeOf 按主型与轮换序号取出当天的练习内容。
// idx 通常由调用方用「年内第几天」对内容条数取模得到，保证每天稳定、跨天轮换。
func DailyPracticeOf(typeID, idx int) (DailyPracticeItem, bool) {
	items, ok := DailyPractices[typeID]
	if !ok || len(items) == 0 {
		return DailyPracticeItem{}, false
	}
	n := idx % len(items)
	if n < 0 {
		n += len(items)
	}
	return items[n], true
}
