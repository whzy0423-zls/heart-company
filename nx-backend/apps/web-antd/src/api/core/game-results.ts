import { requestClient } from '#/api/request';

export interface GameNameValue {
  name: string;
  value: number;
}

export interface GameTypeGenderItem {
  female: number;
  male: number;
  name: string;
  total: number;
  unknown: number;
}

export interface GameOverview {
  centerItems: GameNameValue[];
  genderItems: GameNameValue[];
  total: number;
  typeGenderItems: GameTypeGenderItem[];
  typeItems: GameNameValue[];
}

export function getGameOverviewApi() {
  return requestClient.get<GameOverview>('/game-results/overview');
}
