export function clampPosterPosition(x: number, y: number, width: number, height: number) {
  return {
    x: Math.round(Math.max(0, Math.min(720 - width, x))),
    y: Math.round(Math.max(0, Math.min(1280 - height, y))),
  };
}

export function dragPosterPosition(startX: number, startY: number, deltaX: number, deltaY: number, previewWidth: number, previewHeight: number, width: number, height: number) {
  if (previewWidth <= 0 || previewHeight <= 0) return clampPosterPosition(startX, startY, width, height);
  return clampPosterPosition(startX + deltaX * 720 / previewWidth, startY + deltaY * 1280 / previewHeight, width, height);
}
