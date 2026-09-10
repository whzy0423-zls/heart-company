package caresystem

import (
	"fmt"
	"math"
	"strings"
)

func LabelForLevel(level int) string {
	switch {
	case level <= 3:
		return "状态稳定"
	case level <= 6:
		return "需要关注"
	case level <= 8:
		return "持续关怀"
	default:
		return "高优先级关怀"
	}
}

func TrendFor(current int, baseline *int) Trend {
	if baseline == nil || *baseline == 0 || current == *baseline {
		return TrendStable
	}
	if current > *baseline {
		return TrendUp
	}
	return TrendDown
}

func Score(evidence []Evidence, baseline Baseline) Evaluation {
	result := Evaluation{DataStatus: DataStatusInsufficient, Trend: TrendStable, EvaluationVersion: EvaluationVersion}
	if len(evidence) < 2 {
		return result
	}
	var signals Signals
	var weighted float64
	for _, item := range evidence {
		text := strings.TrimSpace(item.Content)
		if text == "" {
			continue
		}
		signals.MessageCount++
		for _, word := range []string{"压力", "焦虑", "崩溃", "难受", "失眠", "害怕", "孤独", "撑不住", "绝望"} {
			if strings.Contains(text, word) {
				signals.DistressHits++
				weighted += 1.2
			}
		}
		for _, word := range []string{"控制不住", "伤害自己", "不想活", "活不下去", "失控"} {
			if strings.Contains(text, word) {
				signals.LossOfControlHits++
				weighted += 3.0
			}
		}
		for _, word := range []string{"谢谢", "好多了", "平静", "解决", "支持", "陪伴", "休息"} {
			if strings.Contains(text, word) {
				signals.SupportHits++
				weighted -= .6
			}
		}
		for _, word := range []string{"恢复", "轻松", "安心", "有希望"} {
			if strings.Contains(text, word) {
				signals.RecoveryHits++
				weighted -= 1.0
			}
		}
	}
	if signals.MessageCount < 2 {
		return result
	}
	level := int(math.Ceil(1 + weighted/2))
	if signals.LossOfControlHits >= 2 {
		level = 10
	}
	if signals.LossOfControlHits == 0 && level > 8 {
		level = 8
	}
	if level < 1 {
		level = 1
	}
	if level > 10 {
		level = 10
	}
	result.Level = &level
	result.Label = LabelForLevel(level)
	result.Trend = TrendFor(level, baseline.Level)
	result.DataStatus = DataStatusReady
	result.Signals = signals
	result.Summary = fmt.Sprintf("结合近期 %d 条对话，检测到 %d 项压力信号与 %d 项支持/恢复信号。", signals.MessageCount, signals.DistressHits+signals.LossOfControlHits, signals.SupportHits+signals.RecoveryHits)
	return result
}
