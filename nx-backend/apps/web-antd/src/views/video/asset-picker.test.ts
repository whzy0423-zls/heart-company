import { describe, expect, it } from 'vitest';

import type { VideoAsset, VideoAssetType } from '#/api';

import {
  ASSET_PICKER_TYPES,
  canPickAsset,
  filterAllowedAssets,
  getAllowedAssetTypes,
  getInitialPickerType,
  normalizePickerQueryType,
} from './components/AssetPicker.vue';

function asset(input: Pick<VideoAsset, 'id' | 'type'>): VideoAsset {
  return {
    assetId: input.id,
    coverUrl: '',
    createTime: '',
    id: input.id,
    name: input.id,
    remark: '',
    status: 'active',
    type: input.type,
    updateTime: '',
    url: '',
  };
}

describe('asset picker allowed type guards', () => {
  it('uses every picker type when no allow list is provided', () => {
    expect(getAllowedAssetTypes()).toEqual(ASSET_PICKER_TYPES);
    expect(getAllowedAssetTypes([])).toEqual(ASSET_PICKER_TYPES);
  });

  it('resets the selected type to the first allowed type or all when opening', () => {
    expect(getInitialPickerType(['audio', 'video'])).toBe('audio');
    expect(getInitialPickerType(['scene'])).toBe('scene');
    expect(getInitialPickerType()).toBe('');
  });

  it('normalizes stale query types while still allowing the all tab', () => {
    const allowTypes: VideoAssetType[] = ['audio', 'video'];

    expect(normalizePickerQueryType('scene', allowTypes)).toBe('audio');
    expect(normalizePickerQueryType('', allowTypes)).toBe('');
    expect(normalizePickerQueryType('video', allowTypes)).toBe('video');
  });

  it('filters fetched assets to the allowed types', () => {
    const result = filterAllowedAssets(
      [
        asset({ id: 'image-1', type: 'scene' }),
        asset({ id: 'audio-1', type: 'audio' }),
      ],
      ['scene'],
    );

    expect(result.map((item) => item.id)).toEqual(['image-1']);
  });

  it('blocks choosing an asset outside the allowed types', () => {
    expect(
      canPickAsset(asset({ id: 'audio-1', type: 'audio' }), ['scene']),
    ).toBe(false);
    expect(
      canPickAsset(asset({ id: 'scene-1', type: 'scene' }), ['scene']),
    ).toBe(true);
  });
});
