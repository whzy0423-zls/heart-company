import { requestClient } from '#/api/request';

export interface DailyQuizOption {
  id: string;
  label?: string;
  text: string;
  typeWeights?: Record<string, number>;
  weights?: Record<string, number>;
}

export interface DailyQuizQuestion {
  body: string;
  dimension?: string;
  id: number;
  options: DailyQuizOption[];
}

export interface DailyQuizQuestionVersion {
  answeredCount: number;
  createTime: string;
  id: number;
  isActive: boolean;
  modelName: string;
  modelProvider: string;
  operator: string;
  question: DailyQuizQuestion;
  questionId: number;
  replaceReason: string;
  setId: number;
  slotNo: number;
  source: string;
  versionNo: number;
}

export interface DailyQuizSet {
  date: string;
  errorMessage?: string;
  generatedAt: string;
  id: number;
  modelName: string;
  modelProvider: string;
  prompt?: string;
  publishedAt: string;
  pushedAt: string;
  questionIds: number[];
  questions: DailyQuizQuestionVersion[];
  rawResponse?: string;
  source: string;
  status: string;
}

export interface DailyQuizSetGenerateParams {
  date?: string;
}

export interface DailyQuizQuestionReplaceParams {
  reason?: string;
}

export function getDailyQuizSetApi(params?: { date?: string }) {
  return requestClient.get<DailyQuizSet>('/daily-quiz/admin/sets', {
    params,
  });
}

export function getTodayDailyQuizSetApi() {
  return requestClient.get<DailyQuizSet>('/daily-quiz/admin/sets/today');
}

export function generateDailyQuizSetApi(data?: DailyQuizSetGenerateParams) {
  return requestClient.post<DailyQuizSet>(
    '/daily-quiz/admin/sets/generate',
    data ?? {},
  );
}

export function replaceDailyQuizQuestionApi(
  setId: number | string,
  slotNo: number | string,
  data?: DailyQuizQuestionReplaceParams,
) {
  return requestClient.post<DailyQuizSet>(
    `/daily-quiz/admin/sets/${setId}/questions/${slotNo}/replace`,
    data ?? {},
  );
}
