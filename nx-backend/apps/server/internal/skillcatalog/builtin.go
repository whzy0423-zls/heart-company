package skillcatalog

type BuiltinCatalog struct {
	Key         string
	Name        string
	Description string
	IconKey     string
	Categories  []BuiltinCategory
}

type BuiltinCategory struct {
	Key        string
	Name       string
	IconKey    string
	ColorToken string
	Skills     []BuiltinSkill
}

type BuiltinSkill struct {
	Key                string
	Name               string
	Summary            string
	SourceNeeded       bool
	ConditionalRelease bool
}

func LearningGrowthBuiltinCatalog() BuiltinCatalog {
	return BuiltinCatalog{
		Key: "learning-growth-books", Name: "学习成长类书籍", Description: "从学习、关系、生活、思考、人文与科学经典中提炼的独立对话技能。", IconKey: "menu_book",
		Categories: []BuiltinCategory{
			{Key: "learning-growth", Name: "学习与成长", IconKey: "school", ColorToken: "sand", Skills: []BuiltinSkill{
				{Key: "art-of-learning", Name: "学习之道", Summary: "用渐进训练、划小圈和压力恢复提升技能"},
				{Key: "deliberate-practice", Name: "刻意练习（来源待补）", Summary: "来源恢复与重新蒸馏中的技能包", SourceNeeded: true},
				{Key: "how-to-read-a-book", Name: "如何阅读一本书", Summary: "从检视阅读到主题阅读建立深度阅读方法"},
				{Key: "late-start-action", Name: "人生没有太晚的开始", Summary: "把长期搁置的兴趣转成低风险行动"},
				{Key: "lifelong-learning", Name: "一生的学习", Summary: "审视经验、关系与自我认识中的持续学习"},
			}},
			{Key: "self-relationships", Name: "自我与关系", IconKey: "people", ColorToken: "pink", Skills: []BuiltinSkill{
				{Key: "analects-reflections", Name: "于丹《论语》心得", Summary: "从处世、交友、理想与人生主题展开反思"},
				{Key: "guai-momo-tou", Name: "乖，摸摸头", Summary: "理解倾听、陪伴、边界和关系叙事"},
				{Key: "qinmi-guanxi", Name: "亲密关系", Summary: "从沟通、依恋、冲突与修复梳理关系互动"},
				{Key: "social-psychology-myers", Name: "社会心理学", Summary: "分析归因、从众、说服、偏见、助人与冲突"},
				{Key: "sociology-of-human-emotions", Name: "人类情感社会学", Summary: "从互动、角色、文化和结构理解情感"},
				{Key: "your-loneliness-is-glorious", Name: "你的孤独，虽败犹荣（来源待补）", Summary: "来源恢复与重新蒸馏中的技能包", SourceNeeded: true},
			}},
			{Key: "wellbeing-life", Name: "身心与生活", IconKey: "self_improvement", ColorToken: "green", Skills: []BuiltinSkill{
				{Key: "energy-management", Name: "精力管理", Summary: "从身体、情感、思想与精神四维管理精力"},
				{Key: "future-self-no-regrets", Name: "不要让未来的你讨厌现在的自己", Summary: "用未来视角审视今天的行动与选择"},
				{Key: "live-life-your-way", Name: "以自己喜欢的方式过一生", Summary: "澄清价值排序并设计低风险生活实验"},
				{Key: "traditional-chinese-health", Name: "中医养生学", Summary: "在健康信息边界内理解传统养生框架"},
			}},
			{Key: "thinking-decisions", Name: "思考与决策", IconKey: "psychology", ColorToken: "blue", Skills: []BuiltinSkill{
				{Key: "justice-what-is-right", Name: "公正：该如何做是好", Summary: "比较功利、自由、德性与公共善的论证"},
				{Key: "systems-thinking", Name: "系统思考", Summary: "用因果回路、时滞、边界和杠杆点理解复杂问题"},
				{Key: "thinking-techniques", Name: "思考的技术", Summary: "用假设验证、逻辑结构和未来推演解决问题"},
			}},
			{Key: "society-organizations", Name: "社会与组织", IconKey: "account_balance", ColorToken: "warning", Skills: []BuiltinSkill{
				{Key: "american-higher-education-21c-partial", Name: "21世纪美国高等教育（部分文本）", Summary: "在缺页提示下理解美国高等教育制度与改革", ConditionalRelease: true},
				{Key: "crowd-psychology", Name: "乌合之众", Summary: "结合历史语境审视群体、信念与领袖说服"},
				{Key: "liang-jian", Name: "亮剑", Summary: "从文学人物、组织冲突与时代悲剧分析作品"},
				{Key: "three-dimensions-practice", Name: "三度修炼", Summary: "围绕职业成长、案例学习、责任与慎独反思"},
			}},
			{Key: "humanities-aesthetics", Name: "人文与审美", IconKey: "palette", ColorToken: "purple", Skills: []BuiltinSkill{
				{Key: "chinese-academy-studies", Name: "中国书院学", Summary: "理解书院起源、制度与讲学祭祀藏书功能"},
				{Key: "chinese-aesthetics-fifteen-lectures", Name: "中国美学十五讲", Summary: "从生命体验、空灵、心性与境界分析审美"},
				{Key: "chinese-social-maladies", Name: "中国人的病：沈从文社会人生思考", Summary: "细读抽象原则、人格形成与个体价值主题"},
				{Key: "jun-changning-growth-novel", Name: "君长宁的穿越成长与新政叙事", Summary: "分析人物成长、教育、家族、边疆与新政叙事", ConditionalRelease: true},
				{Key: "passing-from-your-world", Name: "从你的全世界路过（来源待补）", Summary: "来源恢复与重新蒸馏中的技能包", SourceNeeded: true},
				{Key: "pilgrimage-hidden-valley", Name: "一个人的朝圣：隐秘山谷叙事", Summary: "分析失落、记忆、投射、行动与人物弧线", ConditionalRelease: true},
				{Key: "renjian-cihua", Name: "人间词话", Summary: "用境界、造境写境和隔与不隔细读诗词"},
			}},
			{Key: "science-technology", Name: "科学与技术", IconKey: "science", ColorToken: "info", Skills: []BuiltinSkill{
				{Key: "brain-and-cognitive-science", Name: "大脑与认知科学", Summary: "理解神经结构、感知、注意、记忆、语言与执行功能"},
				{Key: "god-plays-dice", Name: "上帝掷骰子吗", Summary: "沿证据、预测和实验判别理解量子物理史"},
				{Key: "revival-eastern-science-culture", Name: "东方科学文化的复兴", Summary: "审视还原论、整体论、复杂性与科技社会后果"},
				{Key: "short-history-nearly-everything", Name: "万物简史", Summary: "建立从宇宙、地球、生命到人类史的跨尺度框架"},
				{Key: "through-computer-fog", Name: "穿越计算机的迷雾", Summary: "从开关、逻辑门、处理器到操作系统理解计算机"},
				{Key: "world-history-science-technology", Name: "世界科学技术通史", Summary: "理解科学与技术在世界史中的分离、互动与合流"},
			}},
		},
	}
}
