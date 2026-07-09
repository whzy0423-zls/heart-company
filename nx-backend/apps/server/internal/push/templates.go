package push

import "fmt"

// 预定义推送模板。

func DailyPracticeReminder() Message {
	return Message{
		Title:    "今日成长练习已更新",
		Content:  "新的觉察练习等你完成，花 1 分钟开启今天的成长之旅",
		DeepLink: "/daily",
	}
}

func DailyQuizReminder() Message {
	return Message{
		Title:    "今日画像校准题已准备好",
		Content:  "今天 5 道题等你完成，花 1 分钟让系统更懂你",
		DeepLink: "/daily-quiz",
	}
}

func ReassessmentReady(reportID int64) Message {
	return Message{
		Title:    "你的画像校准报告已生成",
		Content:  "系统已根据最近的校准题和互动信号生成新的画像建议",
		DeepLink: fmt.Sprintf("/reassessment/%d", reportID),
	}
}

func WeeklyReportReady() Message {
	return Message{
		Title:    "你的成长周报已生成",
		Content:  "本周成长概览已就绪，点击查看",
		DeepLink: "/reports",
	}
}

func GrowthTaskReminder(day int) Message {
	return Message{
		Title:    "成长任务提醒",
		Content:  fmt.Sprintf("第 %d 天的成长任务等你完成", day),
		DeepLink: "/tasks",
	}
}

func NewCompatibilityResult() Message {
	return Message{
		Title:    "关系合盘结果已生成",
		Content:  "你的关系合盘分析已完成，点击查看详情",
		DeepLink: "/compatibility",
	}
}
