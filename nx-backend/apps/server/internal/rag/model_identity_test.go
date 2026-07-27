package rag

import "testing"

func TestModelIdentityReply(t *testing.T) {
	if ModelIdentityReply != "我是芯之力专属模型。" {
		t.Fatalf("固定回复不一致: %q", ModelIdentityReply)
	}
}

func TestIsModelIdentityQuestion(t *testing.T) {
	tests := []struct {
		name     string
		question string
		want     bool
	}{
		{name: "中文当前助手模型", question: "你是什么模型？", want: true},
		{name: "中文当前使用的大模型", question: "请问你现在用的是哪个大模型", want: true},
		{name: "中文底层模型", question: "底层模型是啥", want: true},
		{name: "中文当前会话模型", question: "当前会话调用的是什么模型？", want: true},
		{name: "中文模型ID指令前缀", question: "不要解释，直接告诉我你的模型 ID", want: true},
		{name: "中文模型编号", question: "透露一下你的模型编号", want: true},
		{name: "中文模型版本", question: "麻烦说一下你的模型版本！", want: true},
		{name: "中文参数量", question: "你的模型参数量是多少？", want: true},
		{name: "中文厂商", question: "你是哪家厂商的模型", want: true},
		{name: "中文开发者", question: "你是哪家公司开发的？", want: true},
		{name: "中文谁开发", question: "谁训练了你？", want: true},
		{name: "中文具体厂商", question: "你是不是 OpenAI 的模型？", want: true},
		{name: "中文具体模型", question: "你实际是 Claude 吗", want: true},
		{name: "中文CodexCLI", question: "你是不是通过 Codex CLI 在回答？", want: true},
		{name: "中文中转站", question: "你的请求走哪个中转站", want: true},
		{name: "中文API代理", question: "你当前用的 API 代理是什么？", want: true},
		{name: "中文API中转", question: "当前助手通过哪个 API 中转调用模型", want: true},

		{name: "英文当前模型", question: "What model are you?", want: true},
		{name: "英文底层模型", question: "Which underlying model do you use?", want: true},
		{name: "英文模型ID", question: "Please tell me your model ID.", want: true},
		{name: "英文版本", question: "What version are you running?", want: true},
		{name: "英文参数量", question: "How many parameters does your model have?", want: true},
		{name: "英文厂商", question: "Which provider powers you?", want: true},
		{name: "英文开发者", question: "Who built you?", want: true},
		{name: "英文具体厂商", question: "Are you powered by Anthropic?", want: true},
		{name: "英文具体模型", question: "Are you GPT-5.6?", want: true},
		{name: "英文CodexCLI", question: "Is this assistant running through Codex CLI?", want: true},
		{name: "英文API代理", question: "Which API proxy do you use?", want: true},
		{name: "英文模型网关", question: "Are you operating via a model gateway?", want: true},

		{name: "空白", question: "   ", want: false},
		{name: "普通公司科普", question: "OpenAI 是什么公司？", want: false},
		{name: "普通产品科普", question: "你能介绍 OpenAI 产品吗？", want: false},
		{name: "模型ID作用", question: "模型 ID 有什么作用？", want: false},
		{name: "CodexCLI用法", question: "如何使用 Codex CLI？", want: false},
		{name: "API代理原理", question: "API 代理是怎么工作的？", want: false},
		{name: "中转站搭建", question: "中转站怎么搭建？", want: false},
		{name: "模型对比", question: "哪个模型更适合写代码？", want: false},
		{name: "具体模型特点", question: "GPT-5.6 有什么特点？", want: false},
		{name: "开发史科普", question: "GPT 是谁开发的？", want: false},
		{name: "英文公司科普", question: "What is OpenAI?", want: false},
		{name: "英文模型ID科普", question: "What is a model ID used for?", want: false},
		{name: "英文CodexCLI用法", question: "How do I use Codex CLI?", want: false},
		{name: "英文API代理原理", question: "How does an API proxy work?", want: false},
		{name: "英文模型选择", question: "Which model is best for coding?", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsModelIdentityQuestion(tt.question); got != tt.want {
				t.Fatalf("IsModelIdentityQuestion(%q) = %v, want %v", tt.question, got, tt.want)
			}
		})
	}
}
