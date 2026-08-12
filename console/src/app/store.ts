import { create } from 'zustand';
import type { ID } from '../lib/api';

/** UI 态（服务端状态归 TanStack Query） */
interface UiState {
  /** 时间线点击联动：需在图/血缘中高亮的对象 id */
  highlightRef: ID | null;
  setHighlightRef: (id: ID | null) => void;
}

export const useUiStore = create<UiState>((set) => ({
  highlightRef: null,
  setHighlightRef: (id) => set({ highlightRef: id }),
}));
