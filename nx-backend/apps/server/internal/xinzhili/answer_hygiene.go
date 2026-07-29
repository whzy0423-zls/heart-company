package xinzhili

import "nine-xing/nx-backend/apps/server/internal/answerhygiene"

const neutralDirectAnswerFallback = answerhygiene.NeutralDirectAnswerFallback

func isExplicitTechnicalQuestion(question string) bool {
	return answerhygiene.IsExplicitTechnicalQuestion(question)
}

func isProductMetaSentence(sentence string) bool {
	return answerhygiene.IsProductMetaSentence(sentence)
}

func isProductMetaTitle(sentence string) bool {
	return answerhygiene.IsProductMetaTitle(sentence)
}

func isPureImplementationSentence(sentence string) bool {
	return answerhygiene.IsPureImplementationSentence(sentence)
}

type answerSentenceBuffer = answerhygiene.SentenceBuffer
