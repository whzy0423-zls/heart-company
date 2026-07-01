package quiz

import (
	"testing"
)

// scoring_test.go 校验 Go calcType 与 miniapp src/utils/enneagram.js 的 calcType 行为一致：
//   adjusted[id] = score[id] + (score[id]*weight - score[id]) * 0.15
//   排名：调整分降序，同分 id 升序
//   second = 第一个 id != best 且 raw > 0 的型号，无则 0
//   三中心占比：math.Round((centerScore/centerTotal)*100)，centerTotal 为 0 时取 1
// Wing 字段来自 Go 侧 wingOf 规则（JS calcType 不返回 wing），单独断言。

func centerPct(centers []CenterPct, key string) int {
	for _, c := range centers {
		if c.Key == key {
			return c.Pct
		}
	}
	return -1
}

func TestCalcType(t *testing.T) {
	cases := []struct {
		name       string
		gender     string
		score      map[int]int
		wantType   int
		wantSecond int
		wantWing   int
		wantGut    int
		wantHeart  int
		wantHead   int
	}{
		{
			// 中性（gender=""）：权重全 1.0，调整分等于原始分，纯按原始分排名。
			name:   "neutral_mixed",
			gender: "",
			score:  map[int]int{1: 5, 2: 3, 3: 0, 4: 0, 5: 8, 6: 2, 7: 0, 8: 1, 9: 4},
			// 排名 5(8) 1(5) 9(4) 2(3) 6(2) 8(1) 3/4/7(0)
			wantType:   5,
			wantSecond: 1,
			// primary=5：left=4(0) right=6(2) → 0>=2 否 → 6
			wantWing: 6,
			// gut=1,8,9=10 heart=2,3,4=3 head=5,6,7=10 total=23
			wantGut:   43, // round(10/23*100)=43
			wantHeart: 13, // round(3/23*100)=13
			wantHead:  43, // round(10/23*100)=43
		},
		{
			// 男性权重：1↑(1.4) 2↓(0.8)，调整分把 1 顶到 2 之上。
			name:   "male_weight_flip",
			gender: "male",
			score:  map[int]int{1: 10, 2: 10},
			// adjusted[1]=10.6 adjusted[2]=9.7
			wantType:   1,
			wantSecond: 2,
			// primary=1：left=9(0) right=2(10) → 0>=10 否 → 2
			wantWing:  2,
			wantGut:   50, // gut=1=10 heart=2=10 head=0 total=20
			wantHeart: 50,
			wantHead:  0,
		},
		{
			// 女性权重：2↑(1.4) 1↓(0.9)，调整分把 2 顶到 1 之上（与男性相反）。
			name:   "female_weight_flip",
			gender: "female",
			score:  map[int]int{1: 10, 2: 10},
			// adjusted[1]=9.85 adjusted[2]=10.6
			wantType:   2,
			wantSecond: 1,
			// primary=2：left=1(10) right=3(0) → 10>=0 是 → 1
			wantWing:  1,
			wantGut:   50,
			wantHeart: 50,
			wantHead:  0,
		},
		{
			// 仅单型有分：无副型（second=0）。
			name:       "single_type_no_second",
			gender:     "",
			score:      map[int]int{3: 5},
			wantType:   3,
			wantSecond: 0,
			// primary=3：left=2(0) right=4(0) → 0>=0 是 → 2
			wantWing:  2,
			wantGut:   0,
			wantHeart: 100, // heart=3=5 total=5
			wantHead:  0,
		},
		{
			// 全零：调整分全相等，id 升序 → best=1；无 raw>0 → second=0；centerTotal 取 1。
			name:       "all_zero",
			gender:     "",
			score:      map[int]int{},
			wantType:   1,
			wantSecond: 0,
			// primary=1：left=9(0) right=2(0) → 0>=0 是 → 9
			wantWing:  9,
			wantGut:   0,
			wantHeart: 0,
			wantHead:  0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := calcType(tc.score, tc.gender)
			if got.Type != tc.wantType {
				t.Errorf("Type = %d, want %d", got.Type, tc.wantType)
			}
			if got.Second != tc.wantSecond {
				t.Errorf("Second = %d, want %d", got.Second, tc.wantSecond)
			}
			if got.Wing != tc.wantWing {
				t.Errorf("Wing = %d, want %d", got.Wing, tc.wantWing)
			}
			if p := centerPct(got.Centers, "gut"); p != tc.wantGut {
				t.Errorf("gut pct = %d, want %d", p, tc.wantGut)
			}
			if p := centerPct(got.Centers, "heart"); p != tc.wantHeart {
				t.Errorf("heart pct = %d, want %d", p, tc.wantHeart)
			}
			if p := centerPct(got.Centers, "head"); p != tc.wantHead {
				t.Errorf("head pct = %d, want %d", p, tc.wantHead)
			}
		})
	}
}

// TestCalcTypeAdjustedFormula 锁定调整分公式本身（与 JS 逐位一致）。
func TestCalcTypeAdjustedFormula(t *testing.T) {
	got := calcType(map[int]int{1: 10}, "male")
	// adjusted[1] = 10 + (10*1.4-10)*0.15 = 10.6
	if got.Adjusted[1] != 10.6 {
		t.Errorf("adjusted[1] = %v, want 10.6", got.Adjusted[1])
	}
	// 未加权型号调整分等于原始分。
	if got.Adjusted[2] != 0 {
		t.Errorf("adjusted[2] = %v, want 0", got.Adjusted[2])
	}
}
