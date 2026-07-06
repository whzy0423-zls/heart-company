import { requestClient } from '#/api/request';

export interface QuizOption {
  id: string;
  text: string;
  weights?: Record<number | string, number>;
}

export interface QuizQuestion {
  body: string;
  dimension: string;
  id: number;
  options: QuizOption[];
  quizVersion?: string;
  sort: number;
  status: string;
}

export interface QuizQuestionInput {
  body: string;
  dimension: string;
  options: QuizOption[];
  sort: number;
  status: string;
}

export interface QuizCard {
  appUserId: number;
  cardType: string;
  createTime: string;
  id: number;
  mainType: number;
  name: string;
  profile: Record<string, any>;
  relation: string;
  status: string;
  updateTime: string;
  wingType: number;
}

export function getQuizQuestionsApi() {
  return requestClient.get<QuizQuestion[]>('/quiz/questions');
}

export function createQuizQuestionApi(data: QuizQuestionInput) {
  return requestClient.post<QuizQuestion>('/quiz/questions', data);
}

export function updateQuizQuestionApi(id: number | string, data: QuizQuestionInput) {
  return requestClient.put<QuizQuestion>(`/quiz/questions/${id}`, data);
}

export function deleteQuizQuestionApi(id: number | string) {
  return requestClient.delete<boolean>(`/quiz/questions/${id}`);
}

export function getQuizCardsApi(appUserId: number | string) {
  return requestClient.get<QuizCard[]>('/quiz/cards', {
    params: { appUserId },
  });
}
