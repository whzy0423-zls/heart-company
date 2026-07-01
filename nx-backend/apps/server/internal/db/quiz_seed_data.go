package db

// defaultQuizQuestions 九型测评默认题库（18 道情境单选题）。
// 来源：miniapp/src/data/enneagramGame.js 的 QUESTIONS。
// 每个选项 weights 为 typeID(1-9) -> 分值；选项 id 锚定 a/b/c/d。
var defaultQuizQuestions = []quizSeedQuestion{
	{
		Body: "周末突然空出一整天，你最想做的是？",
		Options: []quizSeedOption{
			{ID: "a", Text: "把积压的待办系统地整理清楚，回到井井有条的状态", Weights: map[int]int{1: 2, 6: 1}},
			{ID: "b", Text: "临时约上朋友，去尝点新鲜、热闹一下", Weights: map[int]int{7: 2}},
			{ID: "c", Text: "推进自己的目标，做点能出成果、有进展的事", Weights: map[int]int{3: 2}},
			{ID: "d", Text: "一个人安静地看书 / 钻研感兴趣的领域", Weights: map[int]int{5: 2, 4: 1}},
		},
	},
	{
		Body: "面对一个棘手的问题，你的第一反应通常是？",
		Options: []quizSeedOption{
			{ID: "a", Text: "直接出手，掌控局面、把它解决掉", Weights: map[int]int{8: 2}},
			{ID: "b", Text: "先评估清楚风险，做好万全准备再行动", Weights: map[int]int{6: 2, 1: 1}},
			{ID: "c", Text: "先抽离观察，把背后的原理彻底搞懂", Weights: map[int]int{5: 2}},
			{ID: "d", Text: "找出最正确、最规范的那种做法", Weights: map[int]int{1: 2, 6: 1}},
		},
	},
	{
		Body: "在一个全新的群体里，你更可能是？",
		Options: []quizSeedOption{
			{ID: "a", Text: "主动关心别人，让每个人都感到被照顾", Weights: map[int]int{2: 2}},
			{ID: "b", Text: "保持自己的节奏与风格，不刻意迎合谁", Weights: map[int]int{4: 2, 5: 1}},
			{ID: "c", Text: "很快活跃气氛，成为带动大家的开心果", Weights: map[int]int{7: 2, 3: 1}},
			{ID: "d", Text: "随和地融入，不抢风头也不排斥任何人", Weights: map[int]int{9: 2}},
		},
	},
	{
		Body: "别人最常用哪个词形容你？",
		Options: []quizSeedOption{
			{ID: "a", Text: "靠谱、有原则", Weights: map[int]int{1: 2}},
			{ID: "b", Text: "温暖、热心", Weights: map[int]int{2: 2}},
			{ID: "c", Text: "优秀、能干", Weights: map[int]int{3: 2}},
			{ID: "d", Text: "随和、好相处", Weights: map[int]int{9: 2}},
		},
	},
	{
		Body: "做一个重要决定时，你最先看重的是？",
		Options: []quizSeedOption{
			{ID: "a", Text: "逻辑和事实是否站得住脚", Weights: map[int]int{5: 2}},
			{ID: "b", Text: "我内心真实的感受与共鸣", Weights: map[int]int{4: 2}},
			{ID: "c", Text: "是否安全、可靠、有保障", Weights: map[int]int{6: 2}},
			{ID: "d", Text: "我能不能掌握主动、说了算", Weights: map[int]int{8: 2}},
		},
	},
	{
		Body: "压力很大的时候，你最容易？",
		Options: []quizSeedOption{
			{ID: "a", Text: "对自己和别人都变得更挑剔", Weights: map[int]int{1: 2}},
			{ID: "b", Text: "顾着照顾别人，反而忽略了自己", Weights: map[int]int{2: 2}},
			{ID: "c", Text: "更想用成绩和表现去证明自己", Weights: map[int]int{3: 2}},
			{ID: "d", Text: "想回避，把自己悄悄关起来", Weights: map[int]int{9: 2, 5: 1}},
		},
	},
	{
		Body: "你理想中的生活状态是？",
		Options: []quizSeedOption{
			{ID: "a", Text: "自由自在，充满新鲜与可能", Weights: map[int]int{7: 2}},
			{ID: "b", Text: "有力量，能保护住自己在乎的人", Weights: map[int]int{8: 2}},
			{ID: "c", Text: "内心平和，关系和谐不内耗", Weights: map[int]int{9: 2}},
			{ID: "d", Text: "活出独一无二、真实的自己", Weights: map[int]int{4: 2}},
		},
	},
	{
		Body: "团队合作中，你常扮演的角色是？",
		Options: []quizSeedOption{
			{ID: "a", Text: "盯流程、排查风险、守住底线的人", Weights: map[int]int{6: 2, 1: 1}},
			{ID: "b", Text: "协调关系、照顾大家情绪的人", Weights: map[int]int{2: 2, 9: 1}},
			{ID: "c", Text: "冲在前面、带着大家拿结果的人", Weights: map[int]int{3: 2, 8: 1}},
			{ID: "d", Text: "出点子、提供新思路的人", Weights: map[int]int{7: 2, 5: 1}},
		},
	},
	{
		Body: "当别人向你寻求帮助时，你会？",
		Options: []quizSeedOption{
			{ID: "a", Text: "尽全力去帮，哪怕委屈了自己", Weights: map[int]int{2: 2}},
			{ID: "b", Text: "先评估值不值得、自己能不能帮得上", Weights: map[int]int{5: 2, 8: 1}},
			{ID: "c", Text: "爽快答应，帮到人自己也很开心", Weights: map[int]int{7: 2}},
			{ID: "d", Text: "尽责地帮，但会反复确认稳妥可靠", Weights: map[int]int{6: 2, 2: 1}},
		},
	},
	{
		Body: "面对改变和不确定，你的态度是？",
		Options: []quizSeedOption{
			{ID: "a", Text: "兴奋——机会来了！", Weights: map[int]int{7: 2}},
			{ID: "b", Text: "谨慎——先看清楚风险再说", Weights: map[int]int{6: 2, 5: 1}},
			{ID: "c", Text: "顺其自然，慢慢就适应了", Weights: map[int]int{9: 2}},
			{ID: "d", Text: "我来主导，把不确定变成可控", Weights: map[int]int{8: 2}},
		},
	},
	{
		Body: "你最不能忍受自己？",
		Options: []quizSeedOption{
			{ID: "a", Text: "犯错、不够完美", Weights: map[int]int{1: 2}},
			{ID: "b", Text: "变得平庸、和别人没两样", Weights: map[int]int{4: 2, 5: 1}},
			{ID: "c", Text: "失败、不如别人", Weights: map[int]int{3: 2}},
			{ID: "d", Text: "失控、被人压制", Weights: map[int]int{8: 2}},
		},
	},
	{
		Body: "安静独处时，你的内心更多是？",
		Options: []quizSeedOption{
			{ID: "a", Text: "复盘哪里还能做得更好", Weights: map[int]int{3: 2, 1: 1}},
			{ID: "b", Text: "丰富的情绪与天马行空的想象", Weights: map[int]int{4: 2}},
			{ID: "c", Text: "对各种问题与知识的思考", Weights: map[int]int{5: 2}},
			{ID: "d", Text: "难得的放空与平静", Weights: map[int]int{9: 2}},
		},
	},
	{
		Body: "当你和别人意见冲突时，你通常会？",
		Options: []quizSeedOption{
			{ID: "a", Text: "据理力争，对就是对、错就是错", Weights: map[int]int{1: 2}},
			{ID: "b", Text: "先顾及对方感受，尽量不伤和气", Weights: map[int]int{2: 2}},
			{ID: "c", Text: "退一步，先把事实和逻辑理清楚", Weights: map[int]int{5: 2}},
			{ID: "d", Text: "直接表明立场，该强硬就强硬", Weights: map[int]int{8: 2}},
		},
	},
	{
		Body: "最让你有成就感的瞬间是？",
		Options: []quizSeedOption{
			{ID: "a", Text: "做出了独一无二、有我印记的东西", Weights: map[int]int{4: 2}},
			{ID: "b", Text: "把一件事稳稳办妥、滴水不漏", Weights: map[int]int{6: 2}},
			{ID: "c", Text: "被认可，拿到漂亮的结果", Weights: map[int]int{3: 2}},
			{ID: "d", Text: "体验到新鲜、尽兴、好玩的时刻", Weights: map[int]int{7: 2}},
		},
	},
	{
		Body: "关于金钱和未来，你的态度更接近？",
		Options: []quizSeedOption{
			{ID: "a", Text: "量入为出，规划得清清楚楚", Weights: map[int]int{1: 2}},
			{ID: "b", Text: "愿意为在乎的人花，钱是用来对人好的", Weights: map[int]int{2: 2}},
			{ID: "c", Text: "够用就好，不想为钱太操心", Weights: map[int]int{9: 2}},
			{ID: "d", Text: "留足储备和余地，给自己安全边界", Weights: map[int]int{5: 2}},
		},
	},
	{
		Body: "面对批评，你的第一感受通常是？",
		Options: []quizSeedOption{
			{ID: "a", Text: "触动情绪，会反复回味、放在心上", Weights: map[int]int{4: 2}},
			{ID: "b", Text: "警觉，担心是不是哪里出了问题", Weights: map[int]int{6: 2}},
			{ID: "c", Text: "不服，先顶回去再说", Weights: map[int]int{8: 2}},
			{ID: "d", Text: "在意形象，想赶紧扳回一城", Weights: map[int]int{3: 2}},
		},
	},
	{
		Body: "你表达「在乎一个人」，更习惯？",
		Options: []quizSeedOption{
			{ID: "a", Text: "替他把事情安排妥当、提点中肯建议", Weights: map[int]int{1: 2}},
			{ID: "b", Text: "无微不至地照顾、嘘寒问暖", Weights: map[int]int{2: 2}},
			{ID: "c", Text: "走心地分享真实的感受和自己", Weights: map[int]int{4: 2}},
			{ID: "d", Text: "默默罩着他、替他扛事", Weights: map[int]int{8: 2}},
		},
	},
	{
		Body: "什么最能让你“回血”充电？",
		Options: []quizSeedOption{
			{ID: "a", Text: "独处，安静地待在自己的空间里", Weights: map[int]int{5: 2}},
			{ID: "b", Text: "和信任的人在一起，踏实有依靠", Weights: map[int]int{6: 2}},
			{ID: "c", Text: "不被打扰，舒服地放空发呆", Weights: map[int]int{9: 2}},
			{ID: "d", Text: "出去玩，来点新鲜刺激的活动", Weights: map[int]int{7: 2}},
		},
	},
}
