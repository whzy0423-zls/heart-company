export type BailianCopyFeedbackType = 'error' | 'info' | 'success';

export interface BailianCopyFeedback {
  content: string;
  type: BailianCopyFeedbackType;
}

interface BailianCopyResult {
  lastError: string;
  status: string;
}

export function getBailianCopyFeedback(
  result: BailianCopyResult,
): BailianCopyFeedback {
  if (result.status === 'ready') {
    return {
      content: '已迁移到百炼 Qwen，可到芯之力模型配置选择',
      type: 'success',
    };
  }

  if (result.status === 'failed') {
    return {
      content: result.lastError || '迁移到百炼 Qwen 失败，请稍后重试',
      type: 'error',
    };
  }

  return {
    content: '已提交百炼 Qwen 迁移，正在处理中，请稍后刷新查看状态',
    type: 'info',
  };
}

export function updateCopyingProfileIds(
  currentIds: ReadonlySet<string>,
  profileId: string,
  isCopying: boolean,
) {
  const nextIds = new Set(currentIds);
  if (isCopying) {
    nextIds.add(profileId);
  } else {
    nextIds.delete(profileId);
  }
  return nextIds;
}
