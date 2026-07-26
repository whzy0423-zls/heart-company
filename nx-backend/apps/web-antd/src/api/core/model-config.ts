import { requestClient } from '#/api/request';

/** 模型配置视图（读取）：密钥不回传，仅以 apiKeySet 标记是否已配置。 */
export interface ModelConfigView {
  chat: {
    provider: string;
    apiBase: string;
    apiKeySet: boolean;
    model: string;
    timeoutSeconds: number;
  };
  video: {
    apiBase: string;
    apiKeySet: boolean;
    model: string;
  };
  /** 文生图模型（gpt-image-2 中转）：密钥不回传，仅以 apiKeySet 标记。 */
  image: {
    apiBase: string;
    apiKeySet: boolean;
    model: string;
  };
  /** 视频分析模型：地址 / 密钥 / GroupID 复用语音生成 MiniMax 配置。 */
  analysis: {
    apiBase: string;
    apiKeySet: boolean;
    groupId: string;
    model: string;
  };
  /** 管理端大模型：用于后台自动生成每日题等运营内容。 */
  admin: {
    apiBase: string;
    apiKeySet: boolean;
    groupId?: string;
    model: string;
    provider: string;
    timeoutSeconds: number;
  };
  /** 每日题生成模型：默认继承管理端大模型，可单独覆盖。 */
  dailyQuiz: {
    apiBase: string;
    apiKeySet: boolean;
    groupId?: string;
    model: string;
    provider: string;
    timeoutSeconds: number;
  };
  /** AI 辅助：开关 + 系统提示词（提示词非密钥，明文回显）。 */
  assist: {
    enabled: boolean;
    systemPrompt: string;
  };
  xinzhiliVoice: {
    enabled: boolean;
    asr: {
      provider: string;
      apiBase: string;
      apiKeySet: boolean;
      model: string;
      language: string;
      timeoutSeconds: number;
    };
    tts: {
      provider: string;
      apiBase: string;
      apiKeySet: boolean;
      model: string;
      voice: string;
      speed: number;
      responseFormat: string;
      timeoutSeconds: number;
    };
    interaction: {
      endSilenceMs: number;
      minSpeechMs: number;
      maxTurnSeconds: number;
      autoRelisten: boolean;
      tapToInterrupt: boolean;
    };
    systemPrompt: string;
  };
}

/** 模型配置提交（保存）：apiKey 留空表示不修改既有密钥。 */
export interface ModelConfigPayload {
  chat: {
    provider: string;
    apiBase: string;
    apiKey: string;
    model: string;
    timeoutSeconds: number;
  };
  video: {
    apiBase: string;
    apiKey: string;
    model: string;
  };
  /** 文生图模型（gpt-image-2 中转）：apiKey 留空表示不修改既有密钥。 */
  image: {
    apiBase: string;
    apiKey: string;
    model: string;
  };
  /** 视频分析模型：仅 model 会生效，地址 / 密钥 / GroupID 由服务端复用语音生成 MiniMax 配置。 */
  analysis: {
    apiBase: string;
    apiKey: string;
    groupId: string;
    model: string;
  };
  /** 管理端大模型：apiKey 留空表示不修改既有密钥。 */
  admin: {
    apiBase: string;
    apiKey: string;
    groupId?: string;
    model: string;
    provider: string;
    timeoutSeconds: number;
  };
  /** 每日题生成模型：apiKey 留空表示不修改既有密钥。 */
  dailyQuiz: {
    apiBase: string;
    apiKey: string;
    groupId?: string;
    model: string;
    provider: string;
    timeoutSeconds: number;
  };
  /** AI 辅助：开关 + 系统提示词。enabled 始终回传当前值。 */
  assist: {
    enabled: boolean;
    systemPrompt: string;
  };
  xinzhiliVoice: {
    enabled: boolean;
    asr: {
      provider: string;
      apiBase: string;
      apiKey: string;
      model: string;
      language: string;
      timeoutSeconds: number;
    };
    tts: {
      provider: string;
      apiBase: string;
      apiKey: string;
      model: string;
      voice: string;
      speed: number;
      responseFormat: string;
      timeoutSeconds: number;
    };
    interaction: {
      endSilenceMs: number;
      minSpeechMs: number;
      maxTurnSeconds: number;
      autoRelisten: boolean;
      tapToInterrupt: boolean;
    };
    systemPrompt: string;
  };
}

/** 模型配置局部更新：未提交的配置分区由服务端保留原值。 */
export type ModelConfigUpdatePayload = Partial<ModelConfigPayload>;

/** 读取当前生效的模型配置（对话 / 视频），需登录。 */
export function getModelConfigApi() {
  return requestClient.get<ModelConfigView>('/model-config');
}

/** 保存模型配置（对话 / 视频），返回脱敏后的最新视图。需登录。 */
export function updateModelConfigApi(data: ModelConfigUpdatePayload) {
  return requestClient.put<ModelConfigView>('/model-config', data);
}

/** 对话模型连通性测试结果：仅返回最小探测信息，不含密钥。 */
export interface ChatPingResult {
  ok: boolean;
  message: string;
  latencyMs: number;
  apiBase: string;
  model: string;
}

/**
 * 测试兼容协议对话模型连通性：会发送最小生成探测请求，可能产生少量 Token/额度。需登录。
 * 可传入未保存的对话配置（协议 / 地址 / 密钥 / 模型名 / 超时）以测试当前表单；
 * 留空字段会回退到已保存或环境基线配置。
 */
export function testChatModelApi(data?: ModelConfigPayload['chat']) {
  return requestClient.post<ChatPingResult>('/model-config/test-chat', {
    chat: data ?? {},
  });
}
