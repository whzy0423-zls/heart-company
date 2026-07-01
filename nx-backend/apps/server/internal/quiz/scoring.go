package quiz

import (
	"math"
	"sort"
)

// CenterPct 某中心占比。
type CenterPct struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	Pct  int    `json:"pct"`
}

// DirectionRef 成长/压力方向引用。
type DirectionRef struct {
	Type  int    `json:"type"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// ScoreResult 算分结果（与 miniapp calcType 对齐）。
type ScoreResult struct {
	Type     int             `json:"type"`
	Second   int             `json:"second"`
	Wing     int             `json:"wing"`
	Score    map[int]int     `json:"score"`
	Adjusted map[int]float64 `json:"adjusted"`
	Centers  []CenterPct     `json:"centers"`
}

// Persona 卡片画像（下发给 App，App 不内嵌九型规则）。
type Persona struct {
	MainType   int          `json:"mainType"`
	MainName   string       `json:"mainName"`
	MainEn     string       `json:"mainEn"`
	SecondType int          `json:"secondType,omitempty"`
	SecondName string       `json:"secondName,omitempty"`
	WingType   int          `json:"wingType,omitempty"`
	WingLabel  string       `json:"wingLabel,omitempty"`
	Center     string       `json:"center"`
	CenterName string       `json:"centerName"`
	Color      string       `json:"color"`
	Keywords   string       `json:"keywords"`
	Fear       string       `json:"fear"`
	Desire     string       `json:"desire"`
	Growth     DirectionRef `json:"growth"`
	Stress     DirectionRef `json:"stress"`
	Centers    []CenterPct  `json:"centers,omitempty"`
	Title      string       `json:"title"`
	Summary    string       `json:"summary"`
	Motive     string       `json:"motive"`
	Strengths  []string     `json:"strengths"`
	Challenges []string     `json:"challenges"`
	GrowthText string       `json:"growthText"`
	GenderText string       `json:"genderText,omitempty"`
}

// calcType 移植自 miniapp calcType：原始分 → 性别加权调整分 → 主型/副型 → 三中心占比 → 侧翼。
// rawScore[id] 已由服务端从题库 option.weights 累加得到（不信任客户端传入的分值）。
func calcType(rawScore map[int]int, gender string) ScoreResult {
	score := make(map[int]int, 9)
	for id := 1; id <= 9; id++ {
		score[id] = rawScore[id]
	}

	gw := GenderWeight[gender] // 缺省（其它/未填）为 nil，按 1.0 中性

	adjusted := make(map[int]float64, 9)
	for id := 1; id <= 9; id++ {
		w := 1.0
		if gw != nil {
			if v, ok := gw[id]; ok {
				w = v
			}
		}
		s := float64(score[id])
		adjusted[id] = s + (s*w-s)*0.15
	}

	type rankItem struct {
		id  int
		raw int
		val float64
	}
	ranking := make([]rankItem, 0, 9)
	for id := 1; id <= 9; id++ {
		ranking = append(ranking, rankItem{id: id, raw: score[id], val: adjusted[id]})
	}
	// 调整分降序；同分按 id 升序保持稳定（与 JS 默认排序一致的可复现行为）。
	sort.SliceStable(ranking, func(i, j int) bool {
		if ranking[i].val != ranking[j].val {
			return ranking[i].val > ranking[j].val
		}
		return ranking[i].id < ranking[j].id
	})

	best := ranking[0].id
	second := 0
	for _, r := range ranking {
		if r.id != best && r.raw > 0 {
			second = r.id
			break
		}
	}

	// 三中心占比。
	centerScore := map[string]int{"gut": 0, "heart": 0, "head": 0}
	for id := 1; id <= 9; id++ {
		centerScore[CenterOf(id)] += score[id]
	}
	centerTotal := centerScore["gut"] + centerScore["heart"] + centerScore["head"]
	if centerTotal == 0 {
		centerTotal = 1
	}
	centers := make([]CenterPct, 0, 3)
	for _, key := range []string{"gut", "heart", "head"} {
		centers = append(centers, CenterPct{
			Key:  key,
			Name: Centers[key].Name,
			Pct:  int(math.Round(float64(centerScore[key]) / float64(centerTotal) * 100)),
		})
	}

	return ScoreResult{
		Type:     best,
		Second:   second,
		Wing:     wingOf(best, score),
		Score:    score,
		Adjusted: adjusted,
		Centers:  centers,
	}
}

// wingOf 侧翼 = 相邻两型中原始分较高者（环形：1 的相邻是 9 和 2，9 的相邻是 8 和 1）。
func wingOf(primary int, score map[int]int) int {
	left := primary - 1
	if left < 1 {
		left = 9
	}
	right := primary + 1
	if right > 9 {
		right = 1
	}
	if score[left] >= score[right] {
		return left
	}
	return right
}

// buildPersona 由型号组装画像。secondType<=0 表示无副型；centers 可为 nil（如手动建副卡）。
// gender 为空时不附性别专属文案。
func buildPersona(mainType, secondType, wingType int, centers []CenterPct, gender string) Persona {
	info := TypesInfo[mainType]
	res := TypeResults[mainType]

	p := Persona{
		MainType:   mainType,
		MainName:   info.Name,
		MainEn:     info.En,
		Center:     info.Center,
		CenterName: Centers[info.Center].Name,
		Color:      info.Color,
		Keywords:   info.Keywords,
		Fear:       info.Fear,
		Desire:     info.Desire,
		Growth: DirectionRef{
			Type:  info.Growth,
			Name:  TypesInfo[info.Growth].Name,
			Color: TypesInfo[info.Growth].Color,
		},
		Stress: DirectionRef{
			Type:  info.Stress,
			Name:  TypesInfo[info.Stress].Name,
			Color: TypesInfo[info.Stress].Color,
		},
		Centers:    centers,
		Title:      res.Title,
		Summary:    res.Summary,
		Motive:     res.Motive,
		Strengths:  res.Strengths,
		Challenges: res.Challenges,
		GrowthText: res.Growth,
	}

	if secondType >= 1 && secondType <= 9 {
		p.SecondType = secondType
		p.SecondName = TypesInfo[secondType].Name
	}
	if wingType >= 1 && wingType <= 9 {
		p.WingType = wingType
		for _, w := range info.Wings {
			if w.ID == wingType {
				p.WingLabel = w.Label
				break
			}
		}
	}
	switch gender {
	case "male":
		p.GenderText = res.Male
	case "female":
		p.GenderText = res.Female
	}
	return p
}
