package quiz

// 九型人格元数据：从 miniapp/src/data/enneagramGame.js 移植而来（权威来源）。
// 仅题目内容入库（app_quiz_questions），九型规则/画像文案作为后端常量，
// App 端不内嵌九型规则，统一由后端下发。
//
// 中心：gut 本能(8,9,1) / heart 情感(2,3,4) / head 思维(5,6,7)
// 方向（Enneagram Institute 箭头）：
//   成长(整合)：1→7, 2→4, 3→6, 4→1, 5→8, 6→9, 7→5, 8→2, 9→3
//   压力(解离)：1→4, 2→8, 3→9, 4→2, 5→7, 6→3, 7→1, 8→5, 9→6

// Wing 侧翼。
type Wing struct {
	ID    int    `json:"id"`
	Label string `json:"label"`
}

// TypeInfo 九型基础信息。
type TypeInfo struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	En       string `json:"en"`
	Center   string `json:"center"`
	Color    string `json:"color"`
	Keywords string `json:"keywords"`
	Fear     string `json:"fear"`
	Desire   string `json:"desire"`
	Wings    []Wing `json:"wings"`
	Growth   int    `json:"growth"`
	Stress   int    `json:"stress"`
}

// CenterInfo 三中心信息。
type CenterInfo struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Desc  string `json:"desc"`
	Issue string `json:"issue"`
}

// TypeResult 九型结果文案。
type TypeResult struct {
	Title      string   `json:"title"`
	Summary    string   `json:"summary"`
	Motive     string   `json:"motive"`
	Strengths  []string `json:"strengths"`
	Challenges []string `json:"challenges"`
	Growth     string   `json:"growth"`
	Male       string   `json:"male"`
	Female     string   `json:"female"`
}

// TypesInfo 九型基础信息表（id → TypeInfo）。
var TypesInfo = map[int]TypeInfo{
	1: {ID: 1, Name: "完美型", En: "The Reformer", Center: "gut", Color: "green",
		Keywords: "原则 · 自律 · 追求正确", Fear: "害怕犯错、变坏、被指责", Desire: "渴望正直、完善与平衡",
		Wings: []Wing{{ID: 9, Label: "1w9 理想主义者"}, {ID: 2, Label: "1w2 倡导者"}}, Growth: 7, Stress: 4},
	2: {ID: 2, Name: "助人型", En: "The Helper", Center: "heart", Color: "blue",
		Keywords: "关怀 · 付出 · 渴望被需要", Fear: "害怕不被爱、不被需要", Desire: "渴望被爱与被珍视",
		Wings: []Wing{{ID: 1, Label: "2w1 公仆"}, {ID: 3, Label: "2w3 主人翁"}}, Growth: 4, Stress: 8},
	3: {ID: 3, Name: "成就型", En: "The Achiever", Center: "heart", Color: "red",
		Keywords: "目标 · 效率 · 渴望被认可", Fear: "害怕失败、毫无价值", Desire: "渴望感到有价值、被认可",
		Wings: []Wing{{ID: 2, Label: "3w2 魅力者"}, {ID: 4, Label: "3w4 专家"}}, Growth: 6, Stress: 9},
	4: {ID: 4, Name: "自我型", En: "The Individualist", Center: "heart", Color: "blue",
		Keywords: "独特 · 感性 · 寻找自我", Fear: "害怕没有身份、没有意义", Desire: "渴望找到自我、活出真实",
		Wings: []Wing{{ID: 3, Label: "4w3 贵族"}, {ID: 5, Label: "4w5 波西米亚"}}, Growth: 1, Stress: 2},
	5: {ID: 5, Name: "观察型", En: "The Investigator", Center: "head", Color: "green",
		Keywords: "理性 · 求知 · 保留能量", Fear: "害怕无能、被消耗、被侵入", Desire: "渴望有能力、被理解",
		Wings: []Wing{{ID: 4, Label: "5w4 异端"}, {ID: 6, Label: "5w6 问题解决者"}}, Growth: 8, Stress: 7},
	6: {ID: 6, Name: "忠诚型", En: "The Loyalist", Center: "head", Color: "green",
		Keywords: "忠诚 · 警觉 · 寻求安全", Fear: "害怕失去支持与依靠", Desire: "渴望安全感与确定性",
		Wings: []Wing{{ID: 5, Label: "6w5 捍卫者"}, {ID: 7, Label: "6w7 伙伴"}}, Growth: 9, Stress: 3},
	7: {ID: 7, Name: "活跃型", En: "The Enthusiast", Center: "head", Color: "red",
		Keywords: "乐观 · 多元 · 追求可能", Fear: "害怕被困、被剥夺、痛苦", Desire: "渴望满足、自由与快乐",
		Wings: []Wing{{ID: 6, Label: "7w6 娱乐者"}, {ID: 8, Label: "7w8 现实主义者"}}, Growth: 5, Stress: 1},
	8: {ID: 8, Name: "领袖型", En: "The Challenger", Center: "gut", Color: "red",
		Keywords: "力量 · 掌控 · 保护他人", Fear: "害怕被控制、被伤害", Desire: "渴望掌控自己、不被支配",
		Wings: []Wing{{ID: 7, Label: "8w7 独行者"}, {ID: 9, Label: "8w9 巨熊"}}, Growth: 2, Stress: 5},
	9: {ID: 9, Name: "和平型", En: "The Peacemaker", Center: "gut", Color: "blue",
		Keywords: "包容 · 和谐 · 回避冲突", Fear: "害怕冲突、失去联结", Desire: "渴望内在与外在的安宁",
		Wings: []Wing{{ID: 8, Label: "9w8 仲裁者"}, {ID: 1, Label: "9w1 梦想家"}}, Growth: 3, Stress: 6},
}

// Centers 三中心信息表。
var Centers = map[string]CenterInfo{
	"gut":   {Key: "gut", Name: "本能中心", Desc: "关注行动与掌控（8 · 9 · 1）", Issue: "核心议题是「愤怒 / 掌控」"},
	"heart": {Key: "heart", Name: "情感中心", Desc: "关注关系与形象（2 · 3 · 4）", Issue: "核心议题是「形象 / 羞耻」"},
	"head":  {Key: "head", Name: "思维中心", Desc: "关注思考与安全（5 · 6 · 7）", Issue: "核心议题是「焦虑 / 安全」"},
}

// CenterOf 返回某型号所属中心。
func CenterOf(typeID int) string {
	if t, ok := TypesInfo[typeID]; ok {
		return t.Center
	}
	return ""
}

// GenderWeight 性别加权（male/female → typeId → 权重）。缺省（其它/未填）按 1.0 中性。
var GenderWeight = map[string]map[int]float64{
	"male":   {1: 1.4, 8: 1.4, 5: 1.2, 3: 1.1, 9: 1.0, 6: 1.0, 7: 1.0, 2: 0.8, 4: 0.8},
	"female": {2: 1.4, 4: 1.4, 6: 1.2, 9: 1.1, 7: 1.0, 3: 1.0, 1: 0.9, 5: 0.9, 8: 0.8},
}

// TypeResults 九型测试结果文案（id → TypeResult）。
var TypeResults = map[int]TypeResult{
	1: {
		Title: "完美主义者", Summary: "你追求原则与完善，内心有强烈的是非感。你努力做到最好，但也容易对自己和他人过于苛刻。",
		Motive: "渴望正直、完善与平衡，害怕犯错、变坏、被指责",
		Strengths: []string{"原则性强", "自律高效", "追求卓越", "道德感强"},
		Challenges: []string{"过于挑剔", "难以放松", "压抑情绪", "批判性强"},
		Growth:     "学会接纳不完美，允许自己和他人犯错，找到内心的宁静与喜悦（→7）",
		Male:       "你是追求完美的实干家，内心的批评者时常让你感到压力。试着对自己温柔一些，完成比完美更重要。",
		Female:     "你对自己和他人都有很高的要求，这份严谨令人敬佩。记得偶尔放下标准，享受当下的美好。",
	},
	2: {
		Title: "给予者", Summary: "你温暖、关怀，总能感知他人的需要。你乐于付出，但有时会忘记自己的需求，期待他人的认可与回应。",
		Motive: "渴望被爱与被珍视，害怕不被爱、不被需要",
		Strengths: []string{"善解人意", "富有爱心", "乐于助人", "人际能力强"},
		Challenges: []string{"边界模糊", "压抑自身需求", "渴求认可", "情绪化"},
		Growth:     "学会先照顾好自己，区分真正的给予与期待回报的付出（→4）",
		Male:       "你有着细腻的情感和强烈的同理心，这是你的礼物。注意保持健康的边界，你的需求同样重要。",
		Female:     "你是大家的情感支柱，擅长照顾和支持他人。别忘了，允许自己也被他人照顾。",
	},
	3: {
		Title: "成就者", Summary: "你目标明确、行动力强，天生懂得如何展示自己最好的一面。你渴望成功与认可，有时会迷失在形象塑造中。",
		Motive: "渴望感到有价值、被认可，害怕失败、毫无价值",
		Strengths: []string{"高效执行", "目标导向", "适应力强", "激励他人"},
		Challenges: []string{"过于在意形象", "工作狂倾向", "情感疏离", "虚荣"},
		Growth:     "区分真实的自我与扮演的角色，允许自己展示脆弱与真实（→6）",
		Male:       "你是天生的领导者，对成功有强烈的渴望。停下来问问自己：这真的是我想要的，还是别人期待的？",
		Female:     "你充满魅力和能量，总能出色地完成任务。试着在成就之外，探索真实内心深处的渴望。",
	},
	4: {
		Title: "浪漫主义者", Summary: "你感受深刻、富有创意，渴望活出真实独特的自我。你对美与意义有极高的敏感度，但也容易陷入忧郁与自我怀疑。",
		Motive: "渴望找到自我、活出真实，害怕没有身份、没有意义",
		Strengths: []string{"创意丰富", "情感深度", "真实感人", "审美独特"},
		Challenges: []string{"情绪化", "自我沉溺", "羡慕他人", "感觉自己与众不同但又孤独"},
		Growth:     "将内心的丰富情感转化为行动，走出自我，投入当下的生活（→1）",
		Male:       "你拥有别人羡慕的情感深度和艺术感知力。学会在感受情绪的同时不被淹没，你的独特是礼物而非负担。",
		Female:     "你对美、真实和意义有着天生的感知。在探索内心的旅程中，也记得与外部世界保持连接。",
	},
	5: {
		Title: "思考者", Summary: "你独立、理性，热爱知识与思考。你善于分析，但倾向于退缩观察，保留能量，有时会让人觉得疏远。",
		Motive: "渴望有能力、被理解，害怕无能、被消耗、被侵入",
		Strengths: []string{"深度思考", "客观分析", "专注专精", "独立自主"},
		Challenges: []string{"情感疏离", "过度隔离", "社交回避", "囤积资源"},
		Growth:     "勇敢走出头脑，用行动和热情参与生活，分享自己的知识与内心（→8）",
		Male:       "你的思维深邃而独特，这是你最大的优势。试着更多地走进关系，你的观点和存在对他人非常有价值。",
		Female:     "你有着超强的分析能力和清醒的头脑。学会信任自己的感受，在保护边界的同时，也允许他人靠近。",
	},
	6: {
		Title: "忠诚卫士", Summary: "你忠诚、负责，对潜在风险非常敏感。你渴望安全与确定性，是值得信赖的伙伴，但也容易因为焦虑而犹豫不决。",
		Motive: "渴望安全感与确定性，害怕失去支持与依靠",
		Strengths: []string{"忠诚可靠", "有责任心", "善于预见风险", "团队合作"},
		Challenges: []string{"焦虑多疑", "依赖权威", "优柔寡断", "自我怀疑"},
		Growth:     "相信自己内心的指引，在不确定中培养内在的安全感（→9）",
		Male:       "你对朋友和团队极度忠诚，是大家可以依靠的存在。学会信任自己的判断，你比想象中更有能力。",
		Female:     "你有强烈的责任感和对他人的关怀。在照顾好大家的同时，也给自己一些信任和安全感。",
	},
	7: {
		Title: "冒险家", Summary: "你乐观、充满活力，对生活充满热情和好奇。你享受新体验，但有时会逃避负面情绪和深度承诺。",
		Motive: "渴望满足、自由与快乐，害怕被困、被剥夺、痛苦",
		Strengths: []string{"乐观积极", "创意无限", "多才多艺", "富有感染力"},
		Challenges: []string{"逃避痛苦", "难以专注", "承诺困难", "浅尝辄止"},
		Growth:     "学会在当下扎根，拥抱生活的深度与苦乐，而不仅仅追求刺激（→5）",
		Male:       "你的乐观和热情是周围人的能量源泉。学会慢下来，你会发现深度与专注能带来更大的满足感。",
		Female:     "你为生活带来光彩和可能性，总能看到事情好的一面。允许自己偶尔停下来，深入感受内心的需求。",
	},
	8: {
		Title: "挑战者", Summary: "你力量强大、意志坚定，天生有保护弱者的冲动。你重视掌控与真实，但有时会让人感到强势或难以亲近。",
		Motive: "渴望掌控自己、不被支配，害怕被控制、被伤害",
		Strengths: []string{"果断有力", "保护他人", "直接坦诚", "行动力强"},
		Challenges: []string{"控制欲强", "难以示弱", "冲动易怒", "报复心"},
		Growth:     "学会展示脆弱，用力量去服务和照顾他人，而不仅仅是掌控（→2）",
		Male:       "你的力量和意志令人敬畏。真正的强大包括展示温柔——试着在保护他人的同时，也展示你内心的柔软。",
		Female:     "你有着惊人的力量感和领导力。学会信任他人，适当放下掌控，你会发现更深层的连接与满足。",
	},
	9: {
		Title: "和平使者", Summary: "你温和、包容，天生能让周围的人感到舒适。你重视和谐，但有时会通过回避冲突来迷失自己的声音。",
		Motive: "渴望内在与外在的安宁，害怕冲突、失去联结",
		Strengths: []string{"包容理解", "亲和力强", "善于调解", "内心平静"},
		Challenges: []string{"缺乏主见", "拖延回避", "被动消极", "忽视自身需求"},
		Growth:     "找到并坚持自己真正的优先级，用行动表达内心真实的渴望（→3）",
		Male:       "你有着令人安心的包容力，是大家的稳定力量。学会说出自己的想法和需求，你的声音值得被听见。",
		Female:     "你的温和与包容让周围的人感到被接纳。试着更多地优先自己的感受，你的存在本身就很有价值。",
	},
}
